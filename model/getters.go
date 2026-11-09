package model

import "go.mongodb.org/mongo-driver/mongo"

func (b *BaseModel[T, PT]) Collection() *mongo.Collection {
	return b.collection
}
