package admin

import (
	"embed"
	"net/http"

	"github.com/graph-gophers/graphql-go"
	"github.com/graph-gophers/graphql-go/relay"
)

//go:embed schema.graphql
var schemaFS embed.FS

func NewHTTPHandler(resolver *Resolver) (http.Handler, error) {
	schemaStr, err := schemaFS.ReadFile("schema.graphql")
	if err != nil {
		return nil, err
	}

	schema, err := graphql.ParseSchema(string(schemaStr), resolver)
	if err != nil {
		return nil, err
	}

	return &relay.Handler{Schema: schema}, nil
}
