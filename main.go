package main

import (
	"context"
	"fmt"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
	"github.com/uber/jaeger-client-go"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"time"
)

// 后台地址类似：127.0.0.1:16686

const (
	UDPTransport = "127.0.0.1:6831"
)

func NewJaegerTracer(service string) (opentracing.Tracer, io.Closer) {
	sender, err := jaeger.NewUDPTransport(
		UDPTransport,
		0,
		)
	if err != nil {
		panic(err)
	}
	tracer, closer := jaeger.NewTracer(service, jaeger.NewConstSampler(true), jaeger.NewRemoteReporter(sender))
	return tracer, closer
}

func step1(ctx context.Context)  {
	span, _ := opentracing.StartSpanFromContext(ctx, "step-1")
	defer func() {
		span.SetTag("func", "step1")
		span.Finish()
	}()
	time.Sleep(time.Second * 1)
}

func step2(ctx context.Context)  {
	span, _ := opentracing.StartSpanFromContext(ctx, "step-2")
	defer func() {
		span.SetTag("func", "step2")
		span.Finish()
	}()
	time.Sleep(time.Second * 2)
}

func step3(ctx context.Context)  {
	span, _ := opentracing.StartSpanFromContext(ctx, "step-3")
	defer func() {
		span.SetTag("func", "step3")
		span.Finish()
	}()
	time.Sleep(time.Second * 3)
}

// simulateLocalCall函数模拟的是本地调用，在函数的调用过程中，不存在跨应用的情况
func simulateLocalCall()  {
	tracer, closer := NewJaegerTracer("myapp")
	defer closer.Close()
	opentracing.SetGlobalTracer(tracer)
	rootSpan := tracer.StartSpan("root-span")
	defer rootSpan.Finish()
	ctx := opentracing.ContextWithSpan(context.Background(), rootSpan)
	step1(ctx)
	step2(ctx)
	step3(ctx)
}

func server()  {
	http.HandleFunc("/hi", func(writer http.ResponseWriter, request *http.Request) {
		tracer := opentracing.GlobalTracer()
		wireContext, err := tracer.Extract(
			opentracing.HTTPHeaders,
			opentracing.HTTPHeadersCarrier(request.Header))
		if err != nil {
			log.Fatal(err)
		}

		// 由传递过来的trace-id作为父span
		serverSpan := opentracing.StartSpan(
			"server-two-http-root",
			ext.RPCServerOption(wireContext))

		ctx := opentracing.ContextWithSpan(context.Background(), serverSpan)
		defer serverSpan.Finish()

		handle(ctx)

		writer.Write([]byte("hello world"))
	})

	http.ListenAndServe(":8999", nil)
}

func handle(ctx context.Context)  {
	span, _ := opentracing.StartSpanFromContext(ctx, "server-handle")
	defer span.Finish()
	span.SetTag("func", "handle")
	span.SetBaggageItem("params", `a very long string`)
	span.SetBaggageItem("err", `a error with detail information`)
	time.Sleep(time.Second * 5)
}

func client()  {
	client := &http.Client{}
	req, _ := http.NewRequest("GET","http://localhost:8999/hi",nil)
	tracer, closer := NewJaegerTracer("myapp")
	defer closer.Close()
	opentracing.SetGlobalTracer(tracer)
	crossSpan := tracer.StartSpan("cross-span")
	defer crossSpan.Finish()
	ctx := opentracing.ContextWithSpan(context.Background(), crossSpan)
	// 生成一个请求的span
	clientSpan, clientCtx := opentracing.StartSpanFromContext(ctx, "http-one-req")
	_ = clientCtx
	carrier := opentracing.HTTPHeadersCarrier{}
	err := tracer.Inject(clientSpan.Context(), opentracing.HTTPHeaders, carrier)
	if err != nil {
		panic(err)
	}
	defer clientSpan.Finish()
	// 将当前span的trace-id传递到http header中
	for key, value := range carrier {
		req.Header.Add(key, value[0])
	}

	// 发送请求
	resp, _ := client.Do(req)
	defer resp.Body.Close()
	body, _ := ioutil.ReadAll(resp.Body)
	fmt.Println("body:", string(body))
}

// simulateLocalCall函数模拟的是远程调用，存在请求跨应用的情况
// 向外发出HTTP请求时，在header里带上trace-id来实现，被调用方接收请求参数的时候，从header里取trace_id
func simulateRemoteCall()  {
	go server()
	time.Sleep(time.Millisecond * 500)
	client()
}

func main() {
	simulateLocalCall()
	simulateRemoteCall()
}