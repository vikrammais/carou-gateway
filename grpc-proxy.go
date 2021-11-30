package main

import (
  "carou-gateway/extras"
  _ "carou-gateway/protos"
  "encoding/json"
  "errors"
  "fmt"
  "github.com/carousell/go-utils/ctxutils"
  proto2 "github.com/golang/protobuf/proto"
  "github.com/mwitkow/grpc-proxy/proxy"
  "golang.org/x/net/context"
  "golang.org/x/net/http2"
  "golang.org/x/net/http2/h2c"
  "google.golang.org/grpc"
  "google.golang.org/protobuf/reflect/protoreflect"
  "google.golang.org/protobuf/reflect/protoregistry"
  "io/ioutil"
  "log"
  "net"
  "net/http"
  "os"
  "strings"
  "time"
)

func main() {
  configurationFile := "./config.json"

  args := os.Args[1:]
  if len(args) > 0 {
    configurationFile = args[0]
  }

  config := extras.GetConfiguration(configurationFile)

  listen := ":50051"
  if config.Listen != "" {
    listen = config.Listen
  }

  lis, err := net.Listen("tcp", listen)

  if err != nil {
    log.Fatalf("failed to listen: %v", err)
  }
  //grpcServ := grpc.NewServer()
  httpMux := http.NewServeMux()
  httpMux.HandleFunc("/", home)

  fmt.Printf("Proxy running at %q\n", listen)

  server := GetServer(config)

  //if err := server.Serve(lis); err != nil {
  //  log.Fatalf("failed to serve: %v", err)
  //}

  mixedHandler := newHTTPandGRPCMux(httpMux, server)
  http2Server := &http2.Server{}
  http1Server := &http.Server{Handler: h2c.NewHandler(mixedHandler, http2Server)}
  if err != nil {
    panic(err)
  }

  err = http1Server.Serve(lis)
  if errors.Is(err, http.ErrServerClosed) {
    fmt.Println("server closed")
  } else if err != nil {
    panic(err)
  }
}

func home(w http.ResponseWriter, request *http.Request) {
  requestURI := request.RequestURI
  method := request.Method
  body := request.Body
  backendDetails := getBackendServerDetails(requestURI)
  outgoingMethodName := backendDetails.OutGoingMethodName
  requestProto := backendDetails.RequestProtoType
  responseProto := backendDetails.ResponseProtoType
  serverDetails := backendDetails.Backend
  timeout := backendDetails.Timeout
  //decoder := json.NewDecoder(request.Body)
  bytes, err := ioutil.ReadAll(request.Body)

  pbtype, _ := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(requestProto))
  fmt.Println(pbtype)
  msg := proto2.MessageV1(pbtype.New().Interface())
  json.Unmarshal(bytes, msg)

  conn, err := grpc.DialContext(ctxutils.NewBackgroundContext(context.Background()), serverDetails,
    grpc.WithInsecure(),
    grpc.WithTimeout(time.Duration(timeout)*time.Millisecond))

  outputType, _ := protoregistry.GlobalTypes.FindMessageByName(protoreflect.FullName(responseProto))
  output := proto2.MessageV1(outputType.New().Interface())
  err1 := grpc.Invoke(context.Background(), outgoingMethodName, msg, output, conn)
  if err != nil || err1 != nil{
    fmt.Println(err)
  }
  fmt.Println("uri: ", requestURI)
  fmt.Println("method: ", method)
  fmt.Println("body: ", body)
  fmt.Fprintf(w, "", output)
}

func newHTTPandGRPCMux(httpHand http.Handler, grpcHandler http.Handler) http.Handler {
  return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    if r.ProtoMajor == 2 && strings.HasPrefix(r.Header.Get("content-type"), "application/grpc") {
      grpcHandler.ServeHTTP(w, r)
      return
    }
    httpHand.ServeHTTP(w, r)
  })
}

type HttpHandler struct{}
// implement `ServeHTTP` method on `HttpHandler` struct
func (h HttpHandler) ServeHTTP(res http.ResponseWriter, req *http.Request) {
  // create response binary data
  data := []byte("Hello World!") // slice of bytes
  // write `data` to response
  res.Write(data)
}

func GetServer (config extras.Config) *grpc.Server {
  var opts []grpc.ServerOption

  opts = append(opts, grpc.CustomCodec(proxy.Codec()),
    grpc.UnknownServiceHandler(proxy.TransparentHandler(extras.GetDirector(config))))

  //if config.CertFile != "" && config.KeyFile != "" {
  //  creds, err := credentials.NewServerTLSFromFile(config.CertFile, config.KeyFile)
  //  if err != nil {
  //    grpclog.Fatalf("Failed to generate credentials %v", err)
  //  }
  //  opts = append(opts, grpc.Creds(creds))
  //}

  return grpc.NewServer(opts...)
}

func getBackendServerDetails(incomingURI string) extras.Backend {
  configurationFile := "./config.json"
  config := extras.GetConfiguration(configurationFile)
  for _, backend := range config.Backends {
    if strings.HasPrefix(incomingURI, backend.Filter) {
      return backend
    }
  }
  return extras.Backend{}
}

func validateUser() bool {
  return true
}