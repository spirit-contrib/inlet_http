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

const (
	CTX_HTTP_COOKIES = "CTX_HTTP_COOKIES"
	CTX_HTTP_HEADERS = "CTX_HTTP_HEADERS"

	CMD_HTTP_HEADERS_SET = "CMD_HTTP_HEADERS_SET"
	CMD_HTTP_COOKIES_SET = "CMD_HTTP_COOKIES_SET"
)

type NameValue struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type InletHTTPResponseHandler func(spirit.Payload, http.ResponseWriter, *http.Request)
type InletHTTPErrorResponseHandler func(error, http.ResponseWriter, *http.Request)

type option func(*InletHTTP)

type InletHTTP struct {
	config         Config
	requester      Requester
	graphProvider  GraphProvider
	respHandler    InletHTTPResponseHandler
	errRespHandler InletHTTPErrorResponseHandler
}

func (p *InletHTTP) Option(opts ...option) {
	for _, opt := range opts {
		opt(p)
	}
}

func SetRequester(requester Requester) option {
	return func(f *InletHTTP) {
		f.requester = requester
	}
}

func SetHTTPConfig(httpConfig Config) option {
	return func(f *InletHTTP) {
		f.config = httpConfig
	}
}

func SetGraphProvider(graphProvider GraphProvider) option {
	return func(f *InletHTTP) {
		f.graphProvider = graphProvider
	}
}

func SetResponseHandler(handler InletHTTPResponseHandler) option {
	return func(f *InletHTTP) {
		f.respHandler = handler
	}
}

func SetErrorResponseHandler(handler InletHTTPErrorResponseHandler) option {
	return func(f *InletHTTP) {
		f.errRespHandler = handler
	}
}

func NewInletHTTP(opts ...option) *InletHTTP {
	inletHTTP := new(InletHTTP)
	inletHTTP.Option(opts...)

	if inletHTTP.requester == nil {
		inletHTTP.requester = NewClassicRequester()
	}

	return inletHTTP
}

func (p *InletHTTP) Requester() Requester {
	return p.requester
}

func (p *InletHTTP) Run(handlers ...martini.Handler) {
	if p.graphProvider == nil {
		panic("graph provider is nil")
	}

	if p.requester == nil {
		panic("requester is nil")
	}

	if p.respHandler == nil {
		panic("response handler is nil")
	}

	if p.errRespHandler == nil {
		panic("error response handler is nil")
	}

	m := martini.Classic()

	if handlers != nil {
		for _, handler := range handlers {
			m.Use(handler)
		}
	}

	m.Use(p.handler)

	if p.config.Address != "" {
		m.RunOnAddr(p.config.Address)
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
		p.errRespHandler(err, w, r)
		return
	}

	var mapContent map[string]interface{} = make(map[string]interface{})

	if err = json.Unmarshal(binBody, &mapContent); err != nil {
		err = ERR_UNMARSHAL_HTTP_BODY_FAILED.New(errors.Params{"err": err})
		logs.Error(err)
		p.errRespHandler(err, w, r)
		return
	}

	payload := spirit.Payload{}

	payload.SetContent(mapContent)

	responseChan := make(chan spirit.Payload)
	defer close(responseChan)

	var addrs []spirit.MessageAddress
	if addrs, err = p.graphProvider.GetGraph(r); err != nil {
		logs.Error(err)
		p.errRespHandler(err, w, r)
		return
	}

	msgId := ""
	msgId, err = p.requester.Request(addrs, payload, responseChan)
	if err != nil {
		logs.Error(err)
		p.errRespHandler(err, w, r)
		return
	}

	defer p.requester.OnMessageProcessed(msgId)

	var respPayload spirit.Payload

	respPayload = <-responseChan
	//TODO add recv timeout

	p.writeCookiesAndHeaders(respPayload, w, r)
	p.respHandler(respPayload, w, r)
}

func (p *InletHTTP) writeCookiesAndHeaders(payload spirit.Payload, w http.ResponseWriter, r *http.Request) {
	var err error
	// Cookies
	cmdCookiesSize := payload.GetCommandValueSize(CMD_HTTP_COOKIES_SET)
	cmdCookies := make([]interface{}, cmdCookiesSize)
	for i := 0; i < cmdCookiesSize; i++ {
		cookie := new(http.Cookie)
		cmdCookies[i] = cookie
	}

	if err = payload.GetCommandObjectArray(CMD_HTTP_COOKIES_SET, cmdCookies); err != nil {
		err = ERR_PARSE_COMMAND_TO_OBJECT_FAILED.New(errors.Params{"cmd": CMD_HTTP_COOKIES_SET, "err": err})
		p.errRespHandler(err, w, r)
		return
	}

	for _, cookie := range cmdCookies {
		if c, ok := cookie.(*http.Cookie); ok {
			c.Domain = p.config.Domain
			c.Path = "/"
			http.SetCookie(w, c)
		} else {
			err = ERR_PARSE_COMMAND_TO_OBJECT_FAILED.New(errors.Params{"cmd": CMD_HTTP_COOKIES_SET, "err": "object could not parser to cookies"})
			logs.Error(err)
			p.errRespHandler(err, w, r)
			return
		}
	}

	cmdHeadersSize := payload.GetCommandValueSize(CMD_HTTP_HEADERS_SET)
	cmdHeaders := make([]interface{}, cmdHeadersSize)
	for i := 0; i < cmdHeadersSize; i++ {
		header := new(NameValue)
		cmdHeaders[i] = header
	}

	if err = payload.GetCommandObjectArray(CMD_HTTP_HEADERS_SET, cmdHeaders); err != nil {
		err = ERR_PARSE_COMMAND_TO_OBJECT_FAILED.New(errors.Params{"cmd": CMD_HTTP_HEADERS_SET, "err": err})
		logs.Error(err)
		p.errRespHandler(err, w, r)
		return
	}

	for _, header := range cmdHeaders {
		if nv, ok := header.(*NameValue); ok {
			w.Header().Add(nv.Name, nv.Value)
		} else {
			err = ERR_PARSE_COMMAND_TO_OBJECT_FAILED.New(errors.Params{"cmd": CMD_HTTP_HEADERS_SET, "err": "object could not parser to headers"})
			logs.Error(err)
			p.errRespHandler(err, w, r)
			return
		}
	}
}

func (p *InletHTTP) CallBack(payload *spirit.Payload) (result interface{}, err error) {
	p.OnMessageResponse(*payload)
	return nil, nil
}

func (p *InletHTTP) OnMessageResponse(payload spirit.Payload) {
	if _, e := p.requester.OnMessageReceived(payload); e != nil {
		logs.Error(e)
	}
}
