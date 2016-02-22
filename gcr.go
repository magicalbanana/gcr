// Package gcr ...
package gcr

import (
	"errors"
	"net/http"
	"sync"

	"golang.org/x/net/context"
)

var (
	// channel for fetching the context
	fetchCtx chan fetchCtxMessage
	// channel for setting the context
	setCtx chan setCtxMessage
	// channel for fetching the request context
	fetch chan valueMessage
	// channel for setting the current context
	set chan valueMessage
	// channel for killing the context
	kill chan killMessage
	// started stores a true value if the Start() was called
	started bool
	// channel for triggering a stop or done
	stopped            chan bool
	initOnce           = sync.Once{}
	contexts           map[*http.Request]context.Context
	errGCRnotStarted   = errors.New("GCR needs to start to set any request value")
	errSettingTogcr    = errors.New("failed to set variable to gcr")
	errGettingFromgcr  = errors.New("failed to get variable from gcr")
	errEvictingFromgcr = errors.New("failed to evict context from gcr")
)

const (
	// workerDoneKey global context key for when to end the workers
	workerDoneKey int = iota
	// cancelKey global key for cancel function
	cancelKey
)

// setCtxMessage is the struct for channel communications when setting
type setCtxMessage struct {
	Request   *http.Request
	Context   context.Context
	RespondTo chan error
}

// fetchCtxMessage is the struct for channel communications when fetching
type fetchCtxMessage struct {
	Request   *http.Request
	RespondTo chan context.Context
}

// killMessage is the struct for channel communications when killing or
// stopping
type killMessage struct {
	Request   *http.Request
	RespondTo chan error
}

// valueMessage is the struct for channel communications that holds the values
type valueMessage struct {
	Request   *http.Request
	Key       interface{}
	Value     interface{}
	RespondTo chan interface{}
}

// done is a context.CancelFunc variable to override
var done func()

// Options is the struct that can be used to set the default values when
// starting the gcr
type Options struct {
	BufferSize int
	NumWorkers int
	Context    context.Context
}

// NewOptions is a constructor for the Options struct
func NewOptions(ctx context.Context, numWorkers, bufferSize int) *Options {
	return &Options{
		BufferSize: bufferSize,
		NumWorkers: numWorkers,
		Context:    ctx,
	}
}

// SaneDefaults presets the Options fields with non crazy values for the
// BufferSize, NumWorkers and Context.
func (o *Options) SaneDefaults() *Options {
	if o.BufferSize <= 1 {
		o.BufferSize = 1
	}
	if o.NumWorkers <= 0 {
		o.NumWorkers = 1
	}
	if o.Context == nil {
		o.Context = context.Background()
	}
	return o
}

// Start creates a new gcr. If options are passed it will used the passed
// options when starting the gcr. But if there are no options, it will use the
// values returned when options.SaneDefaults() is called.
func Start(options *Options) {
	if options == nil {
		options = NewOptions(nil, 0, 0)
	}
	options.SaneDefaults()
	startGcr(options.Context, options.NumWorkers, options.BufferSize)
}

// Stop ...
var Stop func()

// ClearContextHandler is wrapper Handler to clear the context after
// wrapped handler runs on a request.
type ClearContextHandler struct {
	h http.Handler
}

// NewClearContextHandler creates a new ClearContextHandler
func NewClearContextHandler(h http.Handler) http.Handler {
	return &ClearContextHandler{h: h}
}

// ServeHTTP is an implementation of http.Handler. This allows us to wrap a
// middlware when starting a mux.
func (hw *ClearContextHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	hw.h.ServeHTTP(w, r)
	RemoveRequest(r)
}

// RemoveRequest takes a request and sends a killMessage to the kill channel to
// remove the request from the stored context.
func RemoveRequest(r *http.Request) error {
	err := make(chan error)
	kill <- killMessage{
		Request:   r,
		RespondTo: err,
	}
	return <-err
}

// GetContext takes a request and sends a fetchCtxMessage to the fetchCtx
// channel to get teh associated context with the given request.
func GetContext(r *http.Request) context.Context {
	ctx := make(chan context.Context)
	fetchCtx <- fetchCtxMessage{
		Request:   r,
		RespondTo: ctx,
	}
	return <-ctx
}

// SetContext takes a request and a context then sends a setCtxMessage to the
// setCtx channel to set the given context with the given request.
func SetContext(r *http.Request, ctx context.Context) error {
	if !started {
		return errGCRnotStarted
	}
	err := make(chan error)
	setCtx <- setCtxMessage{
		Request:   r,
		Context:   ctx,
		RespondTo: err,
	}
	return <-err
}

// Set takes a *http.Request and two interfaces then sends a valueMessage to
// the set channel to store the *http.Request with the given key interface and
// value interface.
func Set(r *http.Request, k interface{}, v interface{}) error {
	if !started {
		return errGCRnotStarted
	}
	response := make(chan interface{})
	set <- valueMessage{
		Request:   r,
		Key:       k,
		Value:     v,
		RespondTo: response,
	}
	if r := <-response; r == nil {
		return errSettingTogcr
	}
	return nil
}

// Get takes a *http.Request and a key interface that then sends a
// valueMessage to the fetch channel to get the stored *http.Request and the
// given key's value. If an error occurs when during the fetch, it returns a
// nil value for the interface and an error.
func Get(r *http.Request, k interface{}) (interface{}, error) {
	response := make(chan interface{})
	fetch <- valueMessage{
		Request:   r,
		Key:       k,
		RespondTo: response,
	}
	value := <-response
	if value == nil {
		return nil, errGettingFromgcr
	}
	return value, nil
}

// startGcr takes context.Context and two int types to start the worker
// channels.
func startGcr(ctx context.Context, workers, bufferSize int) {
	initOnce.Do(func() {
		// set the channels for operations
		fetchCtx = make(chan fetchCtxMessage, bufferSize)
		setCtx = make(chan setCtxMessage, bufferSize)
		fetch = make(chan valueMessage, bufferSize)
		set = make(chan valueMessage, bufferSize)
		kill = make(chan killMessage, bufferSize)
		stopped = make(chan bool, workers)

		contexts = make(map[*http.Request]context.Context)

		started = true

		// set the initial context.Context
		if ctx == nil {
			ctx = context.Background()
		}
		ctx, done = context.WithCancel(ctx)

		// setup our worker pool, collect the channels
		var workerChannels []chan bool
		for i := 0; i < workers; i++ {
			workerChannels = append(workerChannels, make(chan bool))
			go worker(ctx, workerChannels[len(workerChannels)-1])
		}

		go func() {
			// if we get word that the global context should die,
			// stop the workers
			<-ctx.Done()
			for _, halt := range workerChannels {
				halt <- true
			}
		}()

		Stop = func() {
			done()
			for _ = range workerChannels {
				<-stopped
			}
		}
	})
}

// worker takes a context.Context and a chan bool to receive any messages from
// channels to perform the operations when setting, fetching, killing or
// evicting requests from the base context.
func worker(baseCtx context.Context, halt chan bool) {
Loop:
	for {
		select {

		case <-halt:
			// stop the worker
			break Loop

		case message := <-fetchCtx:
			// fetch the only the context
			if ctx, ok := contexts[message.Request]; ok && ctx != nil {
				// if there is a request context, grab the
				// message key from it and reply
				message.RespondTo <- ctx
				continue
			}
			// a context was never created for this request,
			// create a cancel context for the caller
			ctx, cancel := context.WithCancel(baseCtx)
			ctx = context.WithValue(ctx, cancelKey, cancel)
			// set this request context on the global context
			contexts[message.Request] = ctx
			// reply to note set is finished
			message.RespondTo <- ctx

		case message := <-fetch:
			// fetch a value from the context
			if ctx, ok := contexts[message.Request]; ok && ctx != nil {
				// if there is a request context, grab the
				// message key from it and reply
				message.RespondTo <- ctx.Value(message.Key)
				continue
			}
			message.RespondTo <- nil

		case message := <-kill:
			// evict the context from the map
			if v, exists := contexts[message.Request]; exists {
				if ctx, ok := v.(context.Context); ok && ctx != nil {

					// if there is a request context, use
					// it's cancel function to end that
					// context
					if v := ctx.Value(cancelKey); v != nil {
						if cancel, ok := v.(context.CancelFunc); ok {
							cancel()
							<-ctx.Done()
							delete(contexts, message.Request)
						}
					}
				}
			}
			message.RespondTo <- nil

		case message := <-setCtx:
			// set the context outright
			if _, exists := contexts[message.Request]; exists {
				contexts[message.Request] = message.Context
			}
			message.RespondTo <- nil

		case message := <-set:
			// set a value on the context
			// get on the global for the request specific context
			if ctx, ok := contexts[message.Request]; ok && ctx != nil {
				// set the key to the existing context
				ctx = context.WithValue(ctx, message.Key, message.Value)
				contexts[message.Request] = ctx
				message.RespondTo <- ctx
				continue
			}
			ctx, cancel := context.WithCancel(baseCtx)
			// add our value to the request context associated with our key
			ctx = context.WithValue(ctx, message.Key, message.Value)
			// create a cancelable context, with the cancel function on said context
			// for easy access
			ctx = context.WithValue(ctx, cancelKey, cancel)

			// set this request context on the global context
			contexts[message.Request] = ctx
			// reply to note set is finished
			message.RespondTo <- ctx
		}
	}
	stopped <- true
}
