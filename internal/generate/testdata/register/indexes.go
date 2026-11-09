package models

import (
	"context"
	"errors"
)

func (models *Models) EnsureIndexes(c context.Context) error {
	return errors.Join(
		models.Contact.EnsureIndexes(c),
		models.Task.EnsureIndexes(c),
	)
}
