// Package migrations holds the versioned schema. Edit collections in the
// Dashboard during dev (automigrate writes the files for you), commit the
// files, and prod applies them on serve. The initial schema also lives here
// as EnsureCollections so tests can build it without replaying migrations.
package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

// EnsureCollections creates the users auth collection and the todos base
// collection when missing. Idempotent: existing collections are left alone
// (use the Dashboard + generated migrations for changes).
func EnsureCollections(app core.App) error {
	if _, err := app.FindCollectionByNameOrId("users"); err != nil {
		users := core.NewAuthCollection("users")
		// Email is the identity field (PasswordAuth.IdentityFields defaults
		// to ["email"]). No extra fields: keep auth minimal.
		if err := app.Save(users); err != nil {
			return err
		}
	}

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return err
	}

	if _, err := app.FindCollectionByNameOrId("todos"); err != nil {
		todos := core.NewBaseCollection("todos")

		rule := types.Pointer("@request.auth.id != '' && owner = @request.auth.id")
		todos.ListRule = rule
		todos.ViewRule = rule
		todos.CreateRule = rule
		todos.UpdateRule = rule
		todos.DeleteRule = rule

		todos.Fields.Add(
			&core.TextField{
				Name:     "title",
				Required: true,
				Max:      255,
			},
			&core.BoolField{
				Name: "done",
			},
			&core.RelationField{
				Name:          "owner",
				Required:      true,
				MaxSelect:     1,
				CascadeDelete: true,
				CollectionId:  users.Id,
			},
			&core.AutodateField{
				Name:     "created",
				OnCreate: true,
			},
			&core.AutodateField{
				Name:     "updated",
				OnCreate: true,
				OnUpdate: true,
			},
		)
		todos.AddIndex("idx_todos_owner", false, "owner", "")

		if err := app.Save(todos); err != nil {
			return err
		}
	}

	return nil
}
