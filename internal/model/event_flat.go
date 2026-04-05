package model

// EventFlat is a pointer-free representation of Event (V3, V4).
//
// Eliminating pointer fields (string, []string) means the GC does not need
// to scan EventFlat values on the heap, which reduces GC mark time and
// shortens stop-the-world pauses under high allocation rates.
//
// Trade-offs:
//   - ID is limited to 16 bytes (UUID without dashes fits exactly).
//   - Name is represented by an index into an interned string table.
//   - Tags are collapsed into a bitmask (up to 32 distinct tags).
type EventFlat struct {
	Value     float64
	Timestamp int64
	TagMask   uint32
	NameIdx   uint16
	ID        [16]byte
}

// TagNames is the global tag dictionary shared across handlers.
// In a real service this would be populated from configuration.
var TagNames = [32]string{
	"error", "warn", "info", "debug",
	"http", "db", "cache", "queue",
	"auth", "payment", "search", "upload",
	"download", "email", "sms", "push",
	"mobile", "web", "api", "grpc",
	"internal", "external", "partner", "system",
	"critical", "high", "medium", "low",
	"read", "write", "delete", "admin",
}

// TagIndex returns the bitmask position for a given tag name, or -1 if unknown.
func TagIndex(name string) int {
	for i, t := range TagNames {
		if t == name {
			return i
		}
	}
	return -1
}
