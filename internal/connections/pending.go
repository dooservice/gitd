package connections

import (
	"crypto/rand"
	"encoding/hex"
	"sync"
	"time"
)

// pending holds what a connection being made waits on — a manifest sent to GitHub, a code
// a person is to enter — by a random name, until it is finished or expires. A name is good
// once.
type pending[Waiting any] struct {
	guard   sync.Mutex
	waiting map[string]pendingEntry[Waiting]
}

type pendingEntry[Waiting any] struct {
	value   Waiting
	expires time.Time
}

// hold keeps value until expires and returns the name it is kept by.
func (held *pending[Waiting]) hold(value Waiting, expires time.Time) (string, error) {
	random := make([]byte, 24)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	name := hex.EncodeToString(random)
	held.guard.Lock()
	defer held.guard.Unlock()
	if held.waiting == nil {
		held.waiting = map[string]pendingEntry[Waiting]{}
	}
	for other, entry := range held.waiting {
		if !entry.expires.After(time.Now()) {
			delete(held.waiting, other)
		}
	}
	held.waiting[name] = pendingEntry[Waiting]{value: value, expires: expires}
	return name, nil
}

// take is what a name holds, once: it is forgotten as it is taken, and found only before it
// expires.
func (held *pending[Waiting]) take(name string, now time.Time) (Waiting, bool) {
	held.guard.Lock()
	defer held.guard.Unlock()
	entry, found := held.waiting[name]
	delete(held.waiting, name)
	var none Waiting
	if !found || !entry.expires.After(now) {
		return none, false
	}
	return entry.value, true
}
