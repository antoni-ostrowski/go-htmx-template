package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// Initial schema: users auth collection + todos base collection.
// Generated Dashboard edits land in new files; this one stays frozen.
func init() {
	m.Register(func(app core.App) error {
		return EnsureCollections(app)
	}, func(app core.App) error {
		if todos, err := app.FindCollectionByNameOrId("todos"); err == nil {
			if err := app.Delete(todos); err != nil {
				return err
			}
		}
		if users, err := app.FindCollectionByNameOrId("users"); err == nil {
			if err := app.Delete(users); err != nil {
				return err
			}
		}
		return nil
	})
}
