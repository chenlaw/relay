package gateway

import (
	"encoding/json"
	"fmt"
	"github.com/googollee/go-socket.io"
	"github.com/robfig/cron"
	"gopkg.in/googollee/go-engine.io.v1"
	"net/http"
	"reflect"
	"sync"
	"time"
	"github.com/Loopring/relay/log"
	"github.com/Loopring/relay/eventemiter"
)

type BusinessType int

const (
	EventPostfixReq         = "_req"
	EventPostfixRes         = "_res"
	EventPostfixEnd         = "_end"
	DefaultCronSpec3Second  = "0/3 * * * * *"
	DefaultCronSpec10Second = "0/10 * * * * *"
	DefaultCronSpec5Minute  = "0 */5 * * * *"
)

const (
	emitTypeByEvent = 1
	emitTypeByCron = 2

)

type Server struct {
	socketio.Server
}

type SocketIOJsonResp struct {
	Error string      `json:"error"`
	Code  string      `json:"code"`
	Data  interface{} `json:"data"`
}

func NewServer(s socketio.Server) Server {
	return Server{s}
}

func (s Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	OriginList := r.Header["Origin"]
	Origin := ""
	if len(OriginList) > 0 {
		Origin = OriginList[0]
	}
	w.Header().Add("Access-Control-Allow-Origin", Origin)
	//w.Header().Add("Access-Control-Allow-Origin", "http://localhost:8000")
	w.Header().Add("Access-Control-Allow-Credentials", "true")
	//w.Header().Add("Access-Control-Allow-Origin", "*")
	w.Header().Add("Access-Control-Allow-Headers", "accept, origin, content-type")
	w.Header().Add("Access-Control-Allow-Methods", "PUT,POST,GET,DELETE,OPTIONS")
	//w.Header().Add("Content-Type", "application/json;charset=utf-8")
	s.Server.ServeHTTP(w, r)
}

type InvokeInfo struct {
	MethodName  string
	Query       interface{}
	isBroadcast bool
	emitType    int
	spec        string
}

const (
	eventKeyTickers = "tickers"
	eventKeyLoopringTickers = "loopringTickers"
	eventKeyTrends = "trends"
	eventKeyPortfolio = "portfolio"
	eventKeyMarketCap = "marketcap"
	eventKeyBalance = "balance"
	eventKeyTransaction = "transaction"
	eventKeyPendingTx = "pendingTx"
	eventKeyDepth = "depth"
)

var EventTypeRoute = map[string]InvokeInfo{
	eventKeyTickers:         {"GetTickers", SingleMarket{}, true, emitTypeByCron, DefaultCronSpec3Second},
	eventKeyLoopringTickers: {"GetAllMarketTickers", nil, true, emitTypeByEvent, DefaultCronSpec3Second},
	eventKeyTrends:          {"GetTrend", TrendQuery{}, true, emitTypeByEvent, DefaultCronSpec3Second},
	// portfolio has been remove from loopr2
	// eventKeyPortfolio:       {"GetPortfolio", SingleOwner{}, false, emitTypeByEvent, DefaultCronSpec3Second},
	eventKeyPortfolio:       {"GetPortfolio", SingleOwner{}, false, emitTypeByCron, DefaultCronSpec3Second},
	eventKeyMarketCap:       {"GetPriceQuote", PriceQuoteQuery{}, true, emitTypeByCron, DefaultCronSpec5Minute},
	eventKeyBalance:         {"GetBalance", CommonTokenRequest{}, false, emitTypeByEvent, DefaultCronSpec3Second},
	eventKeyTransaction:     {"GetTransactions", TransactionQuery{}, false, emitTypeByEvent, DefaultCronSpec3Second},
	eventKeyPendingTx:       {"GetPendingTransactions", SingleOwner{}, false, emitTypeByEvent, DefaultCronSpec10Second},
	eventKeyDepth:           {"GetDepth", DepthQuery{}, true, emitTypeByEvent, DefaultCronSpec3Second},
}

type SocketIOService interface {
	Start(port string)
	Stop()
}

type SocketIOServiceImpl struct {
	port               string
	walletService      WalletServiceImpl
	connIdMap          sync.Map
	connBusinessKeyMap map[string]socketio.Conn
	cron               *cron.Cron
}

func NewSocketIOService(port string, walletService WalletServiceImpl) *SocketIOServiceImpl {
	so := &SocketIOServiceImpl{}
	so.port = port
	so.walletService = walletService
	so.connBusinessKeyMap = make(map[string]socketio.Conn)
	so.connIdMap = sync.Map{}
	so.cron = cron.New()

	// init event watcher
	loopringTickerWatcher := &eventemitter.Watcher{Concurrent: false, Handle: so.broadcastLoopringTicker}
	eventemitter.On(eventemitter.LoopringTickerUpdated, loopringTickerWatcher)
	trendsWatcher := &eventemitter.Watcher{Concurrent: false, Handle: so.broadcastTrends}
	eventemitter.On(eventemitter.TrendUpdated, trendsWatcher)
	portfolioWatcher := &eventemitter.Watcher{Concurrent: false, Handle: so.handlePortfolioUpdate}
	eventemitter.On(eventemitter.TrendUpdated, portfolioWatcher)
	return so
}

func (so *SocketIOServiceImpl) Start() {
	server, err := socketio.NewServer(&engineio.Options{
		PingInterval: time.Second * 60 * 60,
		PingTimeout:  time.Second * 60 * 60,
	})
	if err != nil {
		log.Fatalf(err.Error())
	}
	server.OnConnect("/", func(s socketio.Conn) error {
		so.connIdMap.Store(s.ID(), s)
		return nil
	})
	server.OnEvent("/", "test", func(s socketio.Conn, msg string) {
		fmt.Println("test:", msg)
		s.Emit("reply", "pong relay msg : "+msg)
		fmt.Println("emit message finished...")
		fmt.Println(s.RemoteAddr())
	})

	for v := range EventTypeRoute {
		aliasOfV := v

		server.OnEvent("/", aliasOfV+EventPostfixReq, func(s socketio.Conn, msg string) {
			context := make(map[string]string)
			if s != nil && s.Context() != nil {
				context = s.Context().(map[string]string)
			}
			context[aliasOfV] = msg
			s.SetContext(context)
			so.connIdMap.Store(s.ID(), s)
			so.EmitNowByEventType(aliasOfV, s, msg)
		})

		server.OnEvent("/", aliasOfV+EventPostfixEnd, func(s socketio.Conn, msg string) {
			if s != nil && s.Context() != nil {
				businesses := s.Context().(map[string]string)
				delete(businesses, aliasOfV)
				s.SetContext(businesses)
			}
		})
	}

	for k, events := range EventTypeRoute {
		copyOfK := k
		spec := events.spec

		if events.emitType == emitTypeByEvent {
			continue
		}

		so.cron.AddFunc(spec, func() {
			so.connIdMap.Range(func(key, value interface{}) bool {
				v := value.(socketio.Conn)
				if v.Context() != nil {
					businesses := v.Context().(map[string]string)
					eventContext, ok := businesses[copyOfK]
					if ok {
						so.EmitNowByEventType(copyOfK, v, eventContext)
					}
				}
				return true
			})
		})
	}

	//so.cron.AddFunc("0/10 * * * * *", func() {
	//
	//	for _, v := range so.connIdMap {
	//		if v.Context() == nil {
	//			continue
	//		} else {
	//			businesses := v.Context().(map[string]string)
	//			if businesses != nil {
	//				for bk, bv := range businesses {
	//					so.EmitNowByEventType(bk, v, bv)
	//				}
	//			}
	//		}
	//	}
	//})
	so.cron.Start()

	server.OnError("/", func(e error) {
		fmt.Println("meet error:", e)
	})

	server.OnDisconnect("/", func(s socketio.Conn, msg string) {
		s.Close()
		so.connIdMap.Delete(s.ID())
		fmt.Println("closed", msg)
	})
	go server.Serve()
	defer server.Close()

	http.Handle("/socket.io/", NewServer(*server))
	log.Info("Serving at localhost: " + so.port)
	log.Fatal(http.ListenAndServe(":"+so.port, nil).Error())
	log.Info("finished listen socket io....")

}

func (so *SocketIOServiceImpl) EmitNowByEventType(bk string, v socketio.Conn, bv string) {
	if invokeInfo, ok := EventTypeRoute[bk]; ok {
		so.handleAfterEmit(bk, invokeInfo.Query, invokeInfo.MethodName, v, bv)
	}
}

func (so *SocketIOServiceImpl) handleWith(eventType string, query interface{}, methodName string, ctx string) string {

	results := make([]reflect.Value, 0)
	var err error

	if query == nil {
		results = reflect.ValueOf(&so.walletService).MethodByName(methodName).Call(nil)
	} else {
		queryType := reflect.TypeOf(query)
		queryClone := reflect.New(queryType)
		err = json.Unmarshal([]byte(ctx), queryClone.Interface())
		if err != nil {
			log.Info("unmarshal error " + err.Error())
			errJson, _ := json.Marshal(SocketIOJsonResp{Error: err.Error()})
			return string(errJson[:])

		}
		params := make([]reflect.Value, 1)
		params[0] = queryClone.Elem()
		results = reflect.ValueOf(&so.walletService).MethodByName(methodName).Call(params)
	}

	res := results[0]
	if results[1].Interface() == nil {
		err = nil
	} else {
		err = results[1].Interface().(error)
	}
	if err != nil {
		errJson, _ := json.Marshal(SocketIOJsonResp{Error: err.Error()})
		return string(errJson[:])
	} else {
		rst := SocketIOJsonResp{Data: res.Interface()}
		b, _ := json.Marshal(rst)
		return string(b[:])
	}
}

func (so *SocketIOServiceImpl) handleAfterEmit(eventType string, query interface{}, methodName string, conn socketio.Conn, ctx string) {
	result := so.handleWith(eventType, query, methodName, ctx)
	conn.Emit(eventType+EventPostfixRes, result)
}

func (so *SocketIOServiceImpl) broadcastLoopringTicker(input eventemitter.EventData) (err error) {

	resp := SocketIOJsonResp{}
	tickers, err := so.walletService.GetTicker(SingleContractVersion{})

	if err != nil {
		resp = SocketIOJsonResp{Error: err.Error()}
	} else {
		resp.Data = tickers
	}

	respJson, _ := json.Marshal(resp)

	so.connIdMap.Range(func(key, value interface{}) bool {
		v := value.(socketio.Conn)
		if v.Context() != nil {
			businesses := v.Context().(map[string]string)
			_, ok := businesses[eventKeyLoopringTickers]
			if ok {
				log.Info("emit loopring ticker info")
				v.Emit(eventKeyLoopringTickers + EventPostfixRes, respJson)
			}
		}
		return true
	})
	return nil
}

func (so *SocketIOServiceImpl) broadcastTrends(input eventemitter.EventData) (err error) {

	req := input.(TrendQuery)
	resp := SocketIOJsonResp{}
	trends, err := so.walletService.GetTrend(req)

	if err != nil {
		resp = SocketIOJsonResp{Error: err.Error()}
	} else {
		resp.Data = trends
	}

	respJson, _ := json.Marshal(resp)

	so.connIdMap.Range(func(key, value interface{}) bool {
		v := value.(socketio.Conn)
		if v.Context() != nil {
			businesses := v.Context().(map[string]string)
			ctx, ok := businesses[eventKeyTrends]

			if ok {
				trendQuery := &TrendQuery{}
				err = json.Unmarshal([]byte(ctx), trendQuery)
				if err != nil {
					log.Error("trend query unmarshal error, " + err.Error())
				} else if req.Market == trendQuery.Market && req.Interval == trendQuery.Interval {
					log.Info("emit trend " + ctx)
					v.Emit(eventKeyTrends + EventPostfixRes, respJson)
				}
			}
		}
		return true
	})
	return nil
}

// portfolio has removed from loopr2
func (so *SocketIOServiceImpl) handlePortfolioUpdate(input eventemitter.EventData) (err error) {
	return nil
}