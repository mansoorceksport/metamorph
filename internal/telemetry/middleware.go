package telemetry

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "metamorph-api"

// FiberMiddleware returns a Fiber middleware that traces HTTP requests
func FiberMiddleware() fiber.Handler {
	tracer := otel.Tracer(tracerName)
	propagator := otel.GetTextMapPropagator()

	return func(c *fiber.Ctx) error {
		// Extract trace context from incoming headers
		ctx := propagator.Extract(c.Context(), propagation.HeaderCarrier(c.GetReqHeaders()))

		// Create span name from route
		spanName := fmt.Sprintf("%s %s", c.Method(), c.Path())

		// Start span
		ctx, span := tracer.Start(ctx, spanName,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(
				attribute.String("http.method", c.Method()),
				attribute.String("http.url", c.OriginalURL()),
				attribute.String("http.route", c.Route().Path),
				attribute.String("http.host", c.Hostname()),
				attribute.String("http.user_agent", c.Get("User-Agent")),
				attribute.String("http.client_ip", c.IP()),
			),
		)
		defer span.End()

		// Store span in context for downstream use
		c.SetUserContext(ctx)

		// Add trace ID to response headers for debugging
		if span.SpanContext().HasTraceID() {
			c.Set("X-Trace-ID", span.SpanContext().TraceID().String())
		}

		// Call next handler
		err := c.Next()

		// Record response status
		statusCode := c.Response().StatusCode()
		span.SetAttributes(
			attribute.Int("http.status_code", statusCode),
			attribute.Int("http.response_content_length", len(c.Response().Body())),
		)

		// Mark span as error if status >= 400
		if statusCode >= 400 {
			span.SetStatus(codes.Error, fmt.Sprintf("HTTP %d", statusCode))
		} else {
			span.SetStatus(codes.Ok, "")
		}

		// Record any error
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}

		return err
	}
}

// SpanFromContext gets the current span from Fiber context
func SpanFromContext(c *fiber.Ctx) trace.Span {
	ctx := c.UserContext()
	return trace.SpanFromContext(ctx)
}

// AddSpanEvent adds an event to the current span
func AddSpanEvent(c *fiber.Ctx, name string, attrs ...attribute.KeyValue) {
	span := SpanFromContext(c)
	span.AddEvent(name, trace.WithAttributes(attrs...))
}

// SetSpanAttribute sets an attribute on the current span
func SetSpanAttribute(c *fiber.Ctx, key string, value string) {
	span := SpanFromContext(c)
	span.SetAttributes(attribute.String(key, value))
}
