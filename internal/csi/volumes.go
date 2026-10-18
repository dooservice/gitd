package csi

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/dooservice/gitd/internal/files"
)

// Kubernetes' names for what kubelet tells an ephemeral volume about its pod.
const (
	contextEphemeral = "csi.storage.k8s.io/ephemeral"
	contextNamespace = "csi.storage.k8s.io/pod.namespace"
	contextPod       = "csi.storage.k8s.io/pod.name"
)

// kubernetesName is what a namespace, a pod or a volume name may be.
var kubernetesName = regexp.MustCompile(`^[a-z0-9]([a-z0-9.-]{0,251}[a-z0-9])?$`)

// volumeID is what a CSI volume ID may be, to be kept as a file name.
var volumeID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)

// Asked is what a pod's volume asks for, from its volume attributes.
type Asked struct {
	Repository string
	Branch     string
	Commit     string
	Tenant     string
	// Name tells apart two gitd volumes of one pod.
	Name      string
	Namespace string
	Pod       string
}

// askedOf reads a volume's attributes, refusing what is not an ephemeral volume of a pod.
func askedOf(attributes map[string]string) (Asked, error) {
	if attributes[contextEphemeral] != "true" {
		return Asked{}, fmt.Errorf("gitd.dev volumes are ephemeral inline volumes of a pod only")
	}
	asked := Asked{
		Repository: attributes["repository"], Branch: attributes["branch"], Commit: attributes["commit"],
		Tenant: attributes["tenant"], Name: attributes["name"],
		Namespace: attributes[contextNamespace], Pod: attributes[contextPod],
	}
	if asked.Repository == "" {
		return Asked{}, fmt.Errorf("a gitd.dev volume needs its repository attribute")
	}
	if asked.Branch != "" && asked.Commit != "" {
		return Asked{}, fmt.Errorf("give a branch or a commit, not both")
	}
	for what, value := range map[string]string{"namespace": asked.Namespace, "pod": asked.Pod} {
		if !kubernetesName.MatchString(value) {
			return Asked{}, fmt.Errorf("the pod's %s %q is not one kubelet gives: is podInfoOnMount set?", what, value)
		}
	}
	for what, value := range map[string]string{"name": asked.Name, "tenant": asked.Tenant} {
		if value != "" && !kubernetesName.MatchString(value) {
			return Asked{}, fmt.Errorf("the %s %q is not a Kubernetes name", what, value)
		}
	}
	return asked, nil
}

// workspacePath is where the workspace of a pod's volume lives, under root:
// `<namespace>/<pod>`, and `<namespace>/<pod>-<name>` for a named one. A platform finds it
// there to publish from it.
func (asked Asked) workspacePath(root string) string {
	leaf := asked.Pod
	if asked.Name != "" {
		leaf += "-" + asked.Name
	}
	return filepath.Join(root, asked.Namespace, leaf)
}

// record is what the plugin keeps of a volume it published, to unpublish it.
type record struct {
	Tenant    string `json:"tenant"`
	Workspace string `json:"workspace"`
}

// Records are the volumes the plugin published, one file each under Dir.
type Records struct {
	Dir string
}

func (records Records) path(id string) (string, error) {
	if !volumeID.MatchString(id) {
		return "", fmt.Errorf("%q is not a volume ID the plugin keeps", id)
	}
	return filepath.Join(records.Dir, id+".json"), nil
}

func (records Records) keep(id string, kept record) error {
	path, err := records.path(id)
	if err != nil {
		return err
	}
	data, err := json.Marshal(kept)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(records.Dir, 0o700); err != nil {
		return err
	}
	return files.WritePrivate(path, data)
}

func (records Records) read(id string) (record, bool) {
	path, err := records.path(id)
	if err != nil {
		return record{}, false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return record{}, false
	}
	var kept record
	return kept, json.Unmarshal(data, &kept) == nil
}

func (records Records) forget(id string) {
	if path, err := records.path(id); err == nil {
		_ = os.Remove(path)
	}
}
