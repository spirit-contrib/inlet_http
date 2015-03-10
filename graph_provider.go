package inlet_http

import (
	"net/http"

	"github.com/gogap/spirit"
)

type GraphProvider interface {
	SetGraph(name string, graph spirit.MessageGraph) GraphProvider
	GetGraph(r *http.Request, body []byte) (graphs map[string]spirit.MessageGraph, err error)
}
