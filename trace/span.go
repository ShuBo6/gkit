package trace

import (
	"context"
	"github.com/songzhibin97/gkit/internal/metadata"
	"go.opentelemetry.io/otel/attribute"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc/peer"
	"google.golang.org/protobuf/proto"
	"net"
	"net/url"
	"strings"
)

func setClientSpan(ctx context.Context, span trace.Span, m interface{}) {
	var attrs []attribute.KeyValue
	var remote string
	var operation string
	var rpcKind string
	if tr, ok := FromClientTransportContext(ctx); ok {
		operation = tr.Operation()
		rpcKind = tr.Kind().String()
		if tr.Kind() == KindHTTP {
			if ht, ok := tr.(*Transport); ok {
				method := ht.Request().Method
				route := ht.PathTemplate()
				path := ht.Request().URL.Path
				attrs = append(attrs, semconv.HTTPMethodKey.String(method))
				attrs = append(attrs, semconv.HTTPRouteKey.String(route))
				attrs = append(attrs, semconv.HTTPTargetKey.String(path))
				remote = ht.Request().Host
			}
		} else if tr.Kind() == KindGRPC {
			remote, _ = parseTarget(tr.Endpoint())
		}
	}
	attrs = append(attrs, semconv.RPCSystemKey.String(rpcKind))
	_, mAttrs := parseFullMethod(operation)
	attrs = append(attrs, mAttrs...)
	if remote != "" {
		attrs = append(attrs, peerAttr(remote)...)
	}
	if p, ok := m.(proto.Message); ok {
		attrs = append(attrs, attribute.Key("send_msg.size").Int(proto.Size(p)))
	}

	span.SetAttributes(attrs...)
}

func setServerSpan(ctx context.Context, span trace.Span, m interface{}) {
	attrs := []attribute.KeyValue{}
	var remote string
	var operation string
	var rpcKind string
	if tr, ok := FromServerTransportContext(ctx); ok {
		operation = tr.Operation()
		rpcKind = tr.Kind().String()
		if tr.Kind() == KindHTTP {
			if ht, ok := tr.(*Transport); ok {
				method := ht.Request().Method
				route := ht.PathTemplate()
				path := ht.Request().URL.Path
				attrs = append(attrs, semconv.HTTPMethodKey.String(method))
				attrs = append(attrs, semconv.HTTPRouteKey.String(route))
				attrs = append(attrs, semconv.HTTPTargetKey.String(path))
				remote = ht.Request().RemoteAddr
			}
		} else if tr.Kind() == KindGRPC {
			if p, ok := peer.FromContext(ctx); ok {
				remote = p.Addr.String()
			}
		}
	}
	attrs = append(attrs, semconv.RPCSystemKey.String(rpcKind))
	_, mAttrs := parseFullMethod(operation)
	attrs = append(attrs, mAttrs...)
	attrs = append(attrs, peerAttr(remote)...)
	if p, ok := m.(proto.Message); ok {
		attrs = append(attrs, attribute.Key("receive_msg.size").Int(proto.Size(p)))
	}
	if md, ok := metadata.FromServerContext(ctx); ok {
		attrs = append(attrs, semconv.PeerServiceKey.String(md.GetValue(serverMark)))
	}

	span.SetAttributes(attrs...)
}

// parseFullMethod returns a span name following the OpenTelemetry semantic
// conventions as well as all applicable span attribute.KeyValue attributes based
// on a gRPC's FullMethod.
func parseFullMethod(fullMethod string) (string, []attribute.KeyValue) {
	name := strings.TrimLeft(fullMethod, "/")
	parts := strings.SplitN(name, "/", 2)
	if len(parts) != 2 {
		// Invalid format, does not follow `/package.service/method`.
		return name, []attribute.KeyValue{attribute.Key("rpc.operation").String(fullMethod)}
	}

	var attrs []attribute.KeyValue
	if service := parts[0]; service != "" {
		attrs = append(attrs, semconv.RPCServiceKey.String(service))
	}
	if method := parts[1]; method != "" {
		attrs = append(attrs, semconv.RPCMethodKey.String(method))
	}
	return name, attrs
}

// peerAttr returns attributes about the peer address.
func peerAttr(addr string) []attribute.KeyValue {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return []attribute.KeyValue(nil)
	}

	if host == "" {
		host = "127.0.0.1"
	}

	return []attribute.KeyValue{
		semconv.NetPeerIPKey.String(host),
		semconv.NetPeerPortKey.String(port),
	}
}

func parseTarget(endpoint string) (address string, err error) {
	var u *url.URL
	u, err = url.Parse(endpoint)
	if err != nil {
		if u, err = url.Parse("http://" + endpoint); err != nil {
			return "", err
		}
		return u.Host, nil
	}
	if len(u.Path) > 1 {
		return u.Path[1:], nil
	}
	return endpoint, nil
}
