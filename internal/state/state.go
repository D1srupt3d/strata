// Package state remembers the hash of what strata last wrote to each file,
// enabling three-way drift detection.
package state

import (
	"encoding/json"
	"os"

	"strata/internal/fsutil"
)

type State struct {
	Files map[string]string `json:"files"` // rel path → sha256 of last-applied content
}

func Load(path string) (State, error) {
	s := State{Files: map[string]string{}}
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return s, nil
	}
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(b, &s); err != nil {
		return s, err
	}
	if s.Files == nil {
		s.Files = map[string]string{}
	}
	return s, nil
}

func (s State) Save(path string) error {
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return fsutil.WriteFileAtomic(path, b, 0o644)
}
