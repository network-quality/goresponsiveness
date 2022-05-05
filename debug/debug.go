package debug

type DebugLevel int8

const (
	NoDebug DebugLevel = iota
	Debug
	Warn
	Error
)

type DebugWithPrefix struct {
	Level  DebugLevel
	Prefix string
}

func NewDebugWithPrefix(level DebugLevel, prefix string) *DebugWithPrefix {
	return &DebugWithPrefix{Level: level, Prefix: prefix}
}

func (d *DebugWithPrefix) String() string {
	return d.Prefix
}

func IsDebug(level DebugLevel) bool {
	return level <= Debug
}

func IsWarn(level DebugLevel) bool {
	return level <= Warn
}

func IsError(level DebugLevel) bool {
	return level <= Error
}
