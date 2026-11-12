package templates

// Todo is the view model for one todo row. Mapped from the PB todos record
// in the todo handlers; templates never touch *core.Record directly.
type Todo struct {
	ID    string
	Title string
	Done  bool
}
