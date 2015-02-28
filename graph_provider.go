package inlet_http

import (
	"net/http"

	"github.com/gogap/spirit"
)

type GraphProvider interface {
	SetGraph(apiName string, addr []spirit.MessageAddress) GraphProvider
	GetGraph(r *http.Request) (address []spirit.MessageAddress, err error)
}
