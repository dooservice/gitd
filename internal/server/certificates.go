package server

import (
	"crypto/tls"
	"os"
	"sync"
	"time"
)

// certificateFiles is a TLS certificate and its key, read from their files and read again
// when the certificate's file changes.
type certificateFiles struct {
	cert, key string
	mu        sync.Mutex
	loaded    *tls.Certificate
	modified  time.Time
}

func (files *certificateFiles) get(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	files.mu.Lock()
	defer files.mu.Unlock()
	info, err := os.Stat(files.cert)
	if err != nil {
		if files.loaded != nil {
			return files.loaded, nil
		}
		return nil, err
	}
	if files.loaded != nil && info.ModTime().Equal(files.modified) {
		return files.loaded, nil
	}
	certificate, err := tls.LoadX509KeyPair(files.cert, files.key)
	if err != nil {
		if files.loaded != nil {
			return files.loaded, nil
		}
		return nil, err
	}
	files.loaded, files.modified = &certificate, info.ModTime()
	return files.loaded, nil
}
