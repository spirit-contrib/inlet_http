package inlet_http

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-martini/martini"
	"github.com/gogap/errors"
	"github.com/gogap/logs"
	"github.com/gogap/spirit"
)

type GraphResponse struct {
	GraphName   string
	RespPayload spirit.Payload
	Error       error
}

const (
	REQUEST_TIMEOUT = 30 * time.Second
)

const (
	CTX_HTTP_COOKIES = "CTX_HTTP_COOKIES"
	CTX_HTTP_HEADERS = "CTX_HTTP_HEADERS"

	CMD_HTTP_HEADERS_SET = "CMD_HTTP_HEADERS_SET"
	CMD_HTTP_COOKIES_SET = "CMD_HTTP_COOKIES_SET"
)

type GraphStat struct {
	GraphName    string `json:"-"`
	RequestCount int64  `json:"request_count"`
	TimeoutCount int64  `json:"timeout_count"`
	ErrorCount   int64  `json:"error_count"`

	TotalTimeCost time.Duration `json:"-"`
	MinTimeCost   time.Duration `json:"-"`
	MaxTimeCost   time.Duration `json:"-"`

	StrTotalTimeCost string `json:"total_time_cost"`
	StrMinTimeCost   string `json:"min_time_cost"`
	StrMaxTimeCost   string `json:"max_time_cost"`

	ErrorRate          string `json:"error_rate"`
	TimeoutRate        string `json:"timeout_rate"`
	TimeCostPerRequest string `json:"time_cost_per_request"`
}

func (p *GraphStat) ReCalc() {
	p.ErrorRate = fmt.Sprintf("%.2f", float64(p.ErrorCount/p.RequestCount))
	p.TimeoutRate = fmt.Sprintf("%.2f", float64(p.TimeoutCount/p.RequestCount))
	p.TimeCostPerRequest = fmt.Sprintf("%.2f", float64(float64(p.TotalTimeCost/time.Millisecond)/float64(p.RequestCount)))
	p.StrTotalTimeCost = fmt.Sprintf("%.2f", float64(p.TotalTimeCost/time.Millisecond))
	p.StrMinTimeCost = fmt.Sprintf("%.2f", float64(p.MinTimeCost/time.Millisecond))
	p.StrMaxTimeCost = fmt.Sprintf("%.2f", float64(p.MaxTimeCost/time.Millisecond))

}

type NameValue struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type InletHTTPRequestPayloadHook func(r *http.Request, graphName string, body []byte, payload *spirit.Payload) (err error)
type InletHTTPResponseHandler func(graphsResponse map[string]GraphResponse, w http.ResponseWriter, r *http.Request)
type InletHTTPErrorResponseHandler func(err error, w http.ResponseWriter, r *http.Request)
type InletHTTPRequestDecoder func(body []byte) (map[string]interface{}, error)

type option func(*InletHTTP)

var (
	grapsStat  map[string]*GraphStat = make(map[string]*GraphStat)
	statLocker sync.Mutex
)

type InletHTTP struct {
	config         Config
	requester      Requester
	graphProvider  GraphProvider
	respHandler    InletHTTPResponseHandler
	errRespHandler InletHTTPErrorResponseHandler
	requestDecoder InletHTTPRequestDecoder
	payloadHook    InletHTTPRequestPayloadHook
	timeout        time.Duration
	timeoutHeader  string

	statChan chan GraphStat
}

func (p *InletHTTP) Option(opts ...option) {
	for _, opt := range opts {
		opt(p)
	}
}

func SetRequestPayloadHook(hook InletHTTPRequestPayloadHook) option {
	return func(f *InletHTTP) {
		f.payloadHook = hook
	}
}

func SetRequestDecoder(decoder InletHTTPRequestDecoder) option {
	return func(f *InletHTTP) {
		f.requestDecoder = decoder
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

func SetTimeout(millisecond int64) option {
	return func(f *InletHTTP) {
		f.timeout = time.Duration(millisecond)
	}
}

func SetTimeoutHeader(header string) option {
	return func(f *InletHTTP) {
		f.timeoutHeader = strings.TrimSpace(header)
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

func statCollector(graphStatChan chan GraphStat) {
	for {
		select {
		case graphStat := <-graphStatChan:
			{
				go func(graphStat GraphStat) {
					statLocker.Lock()
					defer statLocker.Unlock()
					if oldStat, exist := grapsStat[graphStat.GraphName]; !exist {
						graphStat.MinTimeCost = graphStat.TotalTimeCost
						graphStat.MaxTimeCost = graphStat.TotalTimeCost
						graphStat.ReCalc()
						grapsStat[graphStat.GraphName] = &graphStat
					} else {
						oldStat.GraphName += graphStat.GraphName
						oldStat.ErrorCount += graphStat.ErrorCount
						oldStat.RequestCount += graphStat.RequestCount
						oldStat.TotalTimeCost += graphStat.TotalTimeCost
						oldStat.TimeoutCount += graphStat.TimeoutCount

						if graphStat.TotalTimeCost < oldStat.MinTimeCost {
							oldStat.MinTimeCost = graphStat.TotalTimeCost
						} else if graphStat.TotalTimeCost > oldStat.MaxTimeCost {
							oldStat.MaxTimeCost = graphStat.TotalTimeCost
						}

						oldStat.ReCalc()
					}
				}(graphStat)

			}
		case <-time.After(time.Second):
		}
	}
}

func statHandler(w http.ResponseWriter, r *http.Request) {
	if data, e := json.MarshalIndent(grapsStat, " ", "  "); e != nil {
		err := ERR_MARSHAL_STAT_DATA_FAILED.New(errors.Params{"err": e})
		logs.Error(err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.Write(data)
	}
	return
}

func (p *InletHTTP) Run(path string, router func(martini.Router), middlerWares ...martini.Handler) {
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

	if p.requestDecoder == nil {
		panic("request encoder is nil")
	}

	if p.config.Timeout > 0 {
		p.timeout = time.Millisecond * time.Duration(p.config.Timeout)
		p.requester.SetTimeout(p.timeout)
	} else {
		p.timeout = REQUEST_TIMEOUT
	}

	m := martini.Classic()

	m.Group(path, router, middlerWares...)

	if p.config.EnableStat {
		p.statChan = make(chan GraphStat, 1000)
		go statCollector(p.statChan)
		m.Get("/stat/json", statHandler)
	}

	if p.config.Address != "" {
		m.RunOnAddr(p.config.Address)
	} else {
		m.Run()
	}
}

func (p *InletHTTP) Handler(w http.ResponseWriter, r *http.Request) {
	var err error
	var binBody []byte
	if binBody, err = ioutil.ReadAll(r.Body); err != nil {
		err = ERR_READ_HTTP_BODY_FAILED.New(errors.Params{"err": err})
		logs.Error(err)
		p.errRespHandler(err, w, r)
		return
	}

	var mapContent map[string]interface{}

	if mapContent, err = p.requestDecoder(binBody); err != nil {
		err = ERR_UNMARSHAL_HTTP_BODY_FAILED.New(errors.Params{"err": err})
		logs.Error(err)
		p.errRespHandler(err, w, r)
		return
	}

	var graphs map[string]spirit.MessageGraph
	if graphs, err = p.graphProvider.GetGraph(r, binBody); err != nil {
		logs.Error(err)
		p.errRespHandler(err, w, r)
		return
	}

	cookies := map[string]string{}
	reqCookies := r.Cookies()
	if reqCookies != nil && len(reqCookies) > 0 {
		for _, cookie := range reqCookies {
			cookies[cookie.Name] = cookie.Value
		}
	}

	payloads := map[string]*spirit.Payload{}

	for graphName, _ := range graphs {

		payload := new(spirit.Payload)
		payload.SetContent(mapContent)
		payload.SetContext(CTX_HTTP_COOKIES, cookies)

		if p.payloadHook != nil {
			if e := p.payloadHook(r, graphName, binBody, payload); e != nil {
				p.errRespHandler(e, w, r)
				return
			}
		}

		payloads[graphName] = payload
	}

	responseChan := make(chan GraphResponse)

	defer close(responseChan)

	timeout := p.timeout

	if p.timeoutHeader != "" {
		if strTimeout := r.Header.Get(p.timeoutHeader); strTimeout != "" {
			if intTimeout, e := strconv.Atoi(strTimeout); e != nil {
				e = ERR_REQUEST_TIMEOUT_VALUE_FORMAT_WRONG.New(errors.Params{"value": strTimeout})
				logs.Warn(e)
			} else {
				timeout = time.Duration(intTimeout) * time.Millisecond
			}
		}
	}

	sendPayloadFunc := func(requester Requester,
		graphName string,
		graph spirit.MessageGraph,
		payload spirit.Payload,
		responseChan chan GraphResponse,
		statChan chan GraphStat,
		timeout time.Duration) {
		defer func() {
			if err := recover(); err != nil {
				return
			}
		}()

		start := time.Now()

		var grapStat GraphStat

		var errCount, timeoutCout int64 = 0, 0

		respChan := make(chan spirit.Payload)
		errChan := make(chan error)
		defer close(respChan)
		defer close(errChan)

		resp := GraphResponse{GraphName: graphName}

		msgId, err := requester.Request(graph, payload, respChan, errChan)
		if err != nil {
			resp.Error = err
			return
		}

		defer requester.OnMessageProcessed(msgId)

		select {
		case resp.RespPayload = <-respChan:
		case resp.Error = <-errChan:
			{
				errCount = 1
			}
		case <-time.After(time.Duration(timeout)):
			{
				timeoutCout = 1
				resp.Error = ERR_REQUEST_TIMEOUT.New(errors.Params{"graphName": graphName, "msgId": msgId})
			}
		}
		end := time.Now()
		timeCost := end.Sub(start)

		responseChan <- resp

		if statChan != nil {
			grapStat.GraphName = graphName
			grapStat.RequestCount = 1
			grapStat.ErrorCount = errCount
			grapStat.TimeoutCount = timeoutCout
			grapStat.TotalTimeCost = timeCost

			select {
			case statChan <- grapStat:
			case <-time.After(time.Second):
			}
		}
	}

	for graphName, payload := range payloads {
		graph, _ := graphs[graphName]

		go sendPayloadFunc(p.requester, graphName, graph, *payload, responseChan, p.statChan, timeout)
	}

	graphsResponse := map[string]GraphResponse{}

	lenGraph := len(graphs)
	for i := 0; i < lenGraph; i++ {
		select {
		case resp := <-responseChan:
			{
				graphsResponse[resp.GraphName] = resp
			}
		case <-time.After(time.Duration(timeout) + time.Second):
			{
				continue
			}
		}
	}

	for graphName, _ := range graphs {
		if _, exist := graphsResponse[graphName]; !exist {
			err := ERR_REQUEST_TIMEOUT.New(errors.Params{"graphName": graphName})
			resp := GraphResponse{
				GraphName: graphName,
				Error:     err,
			}
			graphsResponse[graphName] = resp
		}
	}

	for _, graphResponse := range graphsResponse {
		p.writeCookiesAndHeaders(graphResponse.RespPayload, w, r)
	}

	p.respHandler(graphsResponse, w, r)

	return
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
	p.OnMessageResponse(payload)
	return nil, nil
}

func (p *InletHTTP) Error(payload *spirit.Payload) (result interface{}, err error) {
	p.OnMessageResponse(payload)
	return nil, nil
}

func (p *InletHTTP) OnMessageResponse(payload *spirit.Payload) {
	p.requester.OnMessageReceived(*payload)
}
