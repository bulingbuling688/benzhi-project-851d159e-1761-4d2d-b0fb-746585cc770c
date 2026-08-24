package ledger

import "fmt"

type CorruptionError struct {
	Path     string
	Sequence int64
	Reason   string
}

func (e *CorruptionError) Error() string {
	return fmt.Sprintf("账本损坏 %s sequence=%d: %s", e.Path, e.Sequence, e.Reason)
}

type SchemaError struct {
	Found     int
	Supported int
	Source    string
}

func (e *SchemaError) Error() string {
	return fmt.Sprintf("%s schemaVersion=%d 不受支持，当前支持 %d", e.Source, e.Found, e.Supported)
}
