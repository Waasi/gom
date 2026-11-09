// Package model is gom's Active Record–style model layer for MongoDB.
//
// A model embeds BaseModel[T, *T] and gets Create, Find, FindOne, FindByID,
// Count, Aggregate, Stream, UpdateByID, DeleteByID, DeleteOne and
// FindOneAndUpdate. Its document type implements IModel. The gom CLI generates both, plus migrations (package migrate).
package model
