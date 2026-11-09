# gom

Active Record–style models and migrations for Go + MongoDB.

- **`github.com/waasi/gom/model`** (library): `BaseModel[T, *T]` gives a model `Create`, `Find` (paginated),
  `FindOne`, `FindByID`, `Count`, `Aggregate`, `Stream`, `UpdateByID`, `DeleteByID`,
  `DeleteOne` and `FindOneAndUpdate`, with validation and `createdAt`/`updatedAt` stamping.
- **`gom generate model`** scaffolds a model package and registers it in your `Models`.
- **`gom generate migration`** + **`gom migrate`** are Rails-style migrations, tracked
  in a `schema_migrations` collection.

## Install

```sh
go install github.com/waasi/gom@latest
```

## Quick start

```sh
cd your-module
gom init                       # gom.json, models/, db/migrations/, cmd/migrate/
go get github.com/waasi/gom@latest

gom g model Invoice title:string! total:int company:ref notes:text? tags:[]string
gom g migration backfill_invoice_totals

gom migrate                    # apply pending migrations
gom migrate status
gom migrate down [n]           # roll back the last n (default 1)
```

`go get` adds the library packages (`github.com/waasi/gom/model`, `.../migrate`) that
generated code imports. `gom migrate` runs `go run ./cmd/migrate` in the module root, connecting with
`MONGO_URI` and `MONGO_DB`. A `.env` file in the module root fills in either if unset.

## Models

`gom g model Invoice ...` writes `models/invoices/`:

| file         | contents                                                                 |
|--------------|--------------------------------------------------------------------------|
| `types.go`   | `Invoice` struct: `ID`, your fields, `CreatedAt`, `UpdatedAt`            |
| `new.go`     | `InvoiceModel` embedding `model.BaseModel[Invoice, *Invoice]`, `New(db)`   |
| `getters.go` | `GetID` / `SetID` / `Bind`                                               |
| `methods.go` | `Validate`, timestamps, and instance methods `Save`, `Reload`, `Delete`  |
| `indexes.go` | `EnsureIndexes`, with an index on each `ref` field                       |
| `queries.go` | where your custom finders go                                             |

It then adds the model to the `Models` struct, `New` and `EnsureIndexes` in
`models/`. These are found by structure (not marker comments), so a hand-written
aggregate works too, and re-running the generator changes nothing.

```go
m := models.New(db)
inv, err := m.Invoice.Create(ctx, &invoices.Invoice{Title: "March", Total: 120})
inv.Total = 150
err = inv.Save(ctx)

page, err := m.Invoice.Find(ctx, bson.M{"companyId": id}, model.FindOptions{Page: 2, Limit: 50})
```

### Field syntax

`name[:type][?|!]`. A field with no type is a string.

| type                                        | Go type                         |
|---------------------------------------------|---------------------------------|
| `string`, `text`                            | `string`                        |
| `int`, `int32`, `int64`                     | same                            |
| `float`                                     | `float64`                       |
| `decimal`                                   | `primitive.Decimal128`          |
| `bool`                                      | `bool`                          |
| `time`                                      | `time.Time`                     |
| `id`                                        | `primitive.ObjectID`            |
| `map`, `any`                                | `map[string]any`, `any`         |
| `ref` (`company:ref`)                       | `CompanyID primitive.ObjectID`, bson `companyId`, indexed |
| `[]type` (`tags:[]string`, `members:[]ref`) | slice                           |
| `{...}` (`address:{street:string!,city}`)   | subdocument struct `InvoiceAddress` |
| `[]{...}` (`lines:[]{product:ref,qty:int!}`) | `[]InvoiceLine`, each element validated |

`?` makes a field optional (pointer + `omitempty`); `!` adds `validate:"required"`.

### Subdocuments

`{...}` declares a typed subdocument, generated as its own struct in `types.go` and
named after the path to it: `address` on `Invoice` becomes `InvoiceAddress`, the
elements of `lines` become `InvoiceLine`, and `geo` inside `address` becomes
`InvoiceAddressGeo`. Inside the braces use the same field syntax, including nested
`{...}`. **Quote the argument**, or the shell expands the braces:

```sh
gom g model Invoice title:string! \
  "address:{street:string!,city,zip:string?,geo:{lat:float,lng:float}}" \
  "billing:{name:string!,vat:string?}?" \
  "lines:[]{product:ref,qty:int!,price:decimal}!"
```

Validation reaches into subdocuments: rules inside `address` are checked, and arrays of
subdocuments get `dive` so every element is checked too. A `ref` inside a subdocument is
indexed by its dotted path (`lines.productId`). Query and update them with ordinary
MongoDB operators: `bson.M{"lines.qty": bson.M{"$gt": 4}}`, `$push`, `$elemMatch`,
`"address.city"`.

`UpdateByID` and `FindOneAndUpdate` accept `$set` / `$setOnInsert` as `bson.M`,
`map[string]any` or `bson.D`, never modify the update you pass, and keep an
`updatedAt`/`createdAt` you set yourself.
Tags use [validator](https://github.com/go-playground/validator); register custom rules
on `model.Validator`.

Flags: `--collection NAME` (default: plural lowerCamel, e.g. `invoiceItems`), `--force`,
`--pretend`, `--skip-register`.

## Migrations

```go
func init() {
	migrate.Register(migrate.Migration{
		Version: 20261108203600,
		Name:    "backfill_invoice_totals",
		Up: func(ctx context.Context, db *mongo.Database) error { ... },
		Down: func(ctx context.Context, db *mongo.Database) error { ... },
	})
}
```

Migrations run in version order. A failed migration is not recorded, so the next run
retries it. Leave `Down` nil for a change that can't be undone: `gom migrate down` then
refuses to roll past it.

## Configuration

`gom.json` in the module root (all keys optional):

```json
{ "models": "models", "migrations": "db/migrations", "runner": "cmd/migrate" }
```

## Development

```sh
go test ./...
```

The tests that need MongoDB skip unless one is available. Set `GOM_TEST_MONGO_URI` to use
a running server, or `GOM_TEST_MONGOD` to the path of a `mongod` binary for the tests to
start themselves.
