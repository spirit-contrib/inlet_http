package inlet_http

import (
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/go-martini/martini"
	"github.com/gogap/errors"
	"github.com/gogap/logs"
	"github.com/gogap/spirit"
)

type InletHttpResponseHandler func(spirit.Payload, http.ResponseWriter, *http.Request)

type InletHTTP struct {
	conf          Config
	requester     Requester
	graphProvider GraphProvider
	respHandler   InletHttpResponseHandler
}

func NewInletHTTP(conf Config, requester Requester, graphProvider GraphProvider, handler InletHttpResponseHandler) *InletHTTP {
	if requester == nil {
		panic("requester is nil")
	}

	if handler == nil {
		panic("handler is nil")
	}

	return &InletHTTP{
		conf:          conf,
		requester:     requester,
		graphProvider: graphProvider,
		respHandler:   handler,
	}
}

func (p *InletHTTP) Run(handlers ...martini.Handler) {
	m := martini.Classic()

	if handlers != nil {
		for _, handler := range handlers {
			m.Use(handler)
		}
	}

	m.Use(p.handler)

	if p.conf.Address != "" {
		m.RunOnAddr(p.conf.Address)
	} else {
		m.Run()
	}
}

func (p *InletHTTP) handler(w http.ResponseWriter, r *http.Request) {
	var err error
	var binBody []byte
	if binBody, err = ioutil.ReadAll(r.Body); err != nil {
		err = ERR_READ_HTTP_BODY_FAILED.New(errors.Params{"err": err})
		logs.Error(err)
		return
	}

	var mapContent map[string]interface{} = make(map[string]interface{})

	if err = json.Unmarshal(binBody, &mapContent); err != nil {
		err = ERR_UNMARSHAL_HTTP_BODY_FAILED.New(errors.Params{"err": err})
		logs.Error(err)
		return
	}

	payload := spirit.Payload{}

	payload.SetContent(mapContent)

	responseChan := make(chan *spirit.Payload)
	defer close(responseChan)

	var addrs []spirit.MessageAddress
	if addrs, err = p.graphProvider.GetGraph(r); err != nil {
		logs.Error(err)
		return
	}

	msgId := ""
	msgId, err = p.requester.Request(addrs, payload, responseChan)
	if err != nil {
		logs.Error(err)
		return
	}

	defer p.requester.OnMessageProcessed(msgId)

	var respPayload *spirit.Payload

	respPayload = <-responseChan
	//TODO add recv timeout

	p.respHandler(*respPayload, w, r)
}

func (p *InletHTTP) MessageResponse(payload *spirit.Payload) (result interface{}, err error) {
	if _, e := p.requester.OnMessageReceived(payload); e != nil {
		logs.Error(e)
	}
	return nil, nil
}
