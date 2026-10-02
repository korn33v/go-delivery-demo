// demo — воспроизводимые команды стенда, без jq/Python/Go на компьютере студента.
package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/segmentio/kafka-go"
	"godelivery/contracts/events"
	"godelivery/internal/platform/config"
)

type client struct {
	http                                *http.Client
	order, payment, fulfillment, broker string
}
type order struct {
	ID, Status string
	PaymentID  string `json:"payment_id"`
	Total      int64
	Sequence   int
}
type fulfillment struct {
	ID, Status string
	Sequence   int
}
type state struct {
	OrderID       string `json:"order_id"`
	PaymentID     string `json:"payment_id"`
	TraceID       string `json:"trace_id"`
	Traceparent   string `json:"traceparent"`
	FailedOrderID string `json:"failed_order_id"`
	FailedTraceID string `json:"failed_trace_id"`
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "FAIL:", err)
		os.Exit(1)
	}
}
func run() error {
	c := client{&http.Client{Timeout: 10 * time.Second}, config.Env("ORDER_URL", "http://localhost:8081"), config.Env("PAYMENT_URL", "http://localhost:8082"), config.Env("FULFILLMENT_URL", "http://localhost:8083"), config.Env("KAFKA_BROKER", "localhost:19092")}
	cmd := "help"
	if len(os.Args) > 1 {
		cmd = os.Args[1]
	}
	switch cmd {
	case "help":
		fmt.Println("demo | create | decline | advance | inspect | verify-failure | observe | health | traffic [count] [interval] | field <name> | json-field <key>")
	case "demo":
		s, err := c.happy(true)
		if err != nil {
			return err
		}
		if err = c.decline(&s, true); err != nil {
			return err
		}
		if err = save(s); err != nil {
			return err
		}
		fmt.Printf("SUCCESS: happy + failure paths verified in REST, SQL, Kafka\norder_id=%s\nfailed_order_id=%s\ntrace: http://localhost:16686/trace/%s\n", s.OrderID, s.FailedOrderID, s.TraceID)
	case "create":
		s, err := c.create("tok-success", 201, true)
		if err != nil {
			return err
		}
		return save(s)
	case "remember":
		return remember(false)
	case "remember-failure":
		return remember(true)
	case "wait-status":
		s, err := load()
		if err != nil {
			return err
		}
		base, path := c.order, "/api/v1/orders/"
		if arg(2) == "fulfillment" {
			base, path = c.fulfillment, "/api/v1/fulfillments/"
		}
		return c.waitStatus(base+path+s.OrderID, arg(3))
	case "decline":
		s, _ := load()
		if err := c.decline(&s, true); err != nil {
			return err
		}
		return save(s)
	case "advance":
		s, err := load()
		if err != nil {
			return err
		}
		var f fulfillment
		code, _, err := c.request("POST", c.fulfillment+"/api/v1/fulfillments/"+s.OrderID+"/advance", nil, s.Traceparent, &f)
		if err != nil {
			return err
		}
		if code != 200 {
			return fmt.Errorf("advance HTTP %d", code)
		}
		pretty(f)
	case "inspect":
		return c.inspect()
	case "verify-failure":
		s, err := load()
		if err != nil {
			return err
		}
		return c.verifyFailure(s.FailedOrderID)
	case "observe":
		s, err := load()
		if err != nil {
			return err
		}
		return c.observe(s)
	case "health":
		return c.health()
	case "traffic":
		count := 30
		interval := 2 * time.Second
		if len(os.Args) > 2 {
			n, e := strconv.Atoi(os.Args[2])
			if e != nil || n < 1 {
				return errors.New("traffic count must be positive")
			}
			count = n
		}
		if len(os.Args) > 3 {
			d, e := time.ParseDuration(os.Args[3])
			if e != nil {
				return e
			}
			interval = d
		}
		for i := 0; i < count; i++ {
			if i%4 == 3 {
				s := state{}
				if err := c.decline(&s, false); err != nil {
					return err
				}
			} else {
				if _, err := c.happy(false); err != nil {
					return err
				}
			}
			fmt.Printf("traffic %d/%d\n", i+1, count)
			time.Sleep(interval)
		}
		fmt.Println("SUCCESS: traffic generated")
	case "field":
		s, err := load()
		if err != nil {
			return err
		}
		b, _ := json.Marshal(s)
		return field(bytes.NewReader(b), arg(2))
	case "json-field":
		return field(os.Stdin, arg(2))
	default:
		return fmt.Errorf("unknown command %s", cmd)
	}
	return nil
}
func arg(i int) string {
	if len(os.Args) > i {
		return os.Args[i]
	}
	return ""
}
func statePath() string { return config.Env("DEMO_STATE_FILE", ".demo/state.json") }

// Вспомогательный parser запоминает РЕАЛЬНЫЙ ручной curl, не создаёт новый заказ.
func remember(failed bool) error {
	stem := "checkout"
	if failed {
		stem = "checkout-failed"
	}
	base := filepath.Dir(statePath())
	b, err := os.ReadFile(filepath.Join(base, stem+".json"))
	if err != nil {
		return err
	}
	var o order
	if failed {
		var f struct{ Order order }
		if err = json.Unmarshal(b, &f); err != nil {
			return err
		}
		o = f.Order
	} else {
		if err = json.Unmarshal(b, &o); err != nil {
			return err
		}
	}
	if o.ID == "" {
		return errors.New("curl did not return order ID")
	}
	h, err := os.ReadFile(filepath.Join(base, stem+".headers"))
	if err != nil {
		return err
	}
	headers := http.Header{}
	for _, line := range strings.Split(string(h), "\n") {
		parts := strings.SplitN(line, ":", 2)
		if len(parts) == 2 {
			headers.Set(strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]))
		}
	}
	s, _ := load()
	if failed {
		s.FailedOrderID = o.ID
		s.FailedTraceID = headers.Get("X-Trace-ID")
	} else {
		s.OrderID = o.ID
		s.PaymentID = o.PaymentID
		s.TraceID = headers.Get("X-Trace-ID")
		s.Traceparent = headers.Get("X-Workflow-Traceparent")
	}
	pretty(s)
	return save(s)
}
func save(s state) error {
	if err := os.MkdirAll(filepath.Dir(statePath()), 0755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(s, "", "  ")
	return os.WriteFile(statePath(), b, 0644)
}
func load() (s state, err error) {
	b, err := os.ReadFile(statePath())
	if err != nil {
		return s, fmt.Errorf("run make demo or make checkout first: %w", err)
	}
	err = json.Unmarshal(b, &s)
	return s, err
}
func field(r io.Reader, key string) error {
	var v map[string]any
	if err := json.NewDecoder(r).Decode(&v); err != nil {
		return err
	}
	x, ok := v[key]
	if !ok {
		return fmt.Errorf("field %s is absent", key)
	}
	fmt.Println(x)
	return nil
}
func pretty(x any) { b, _ := json.MarshalIndent(x, "", "  "); fmt.Println(string(b)) }
func (c *client) request(method, address string, input any, parent string, out any) (int, http.Header, error) {
	var body io.Reader
	if input != nil {
		b, err := json.Marshal(input)
		if err != nil {
			return 0, nil, err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, address, body)
	if err != nil {
		return 0, nil, err
	}
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if parent != "" {
		req.Header.Set("traceparent", parent)
	}
	r, err := c.http.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer r.Body.Close()
	if out != nil {
		if err = json.NewDecoder(r.Body).Decode(out); err != nil {
			return r.StatusCode, r.Header, err
		}
	}
	return r.StatusCode, r.Header, nil
}
func input(token string) map[string]any {
	return map[string]any{"customer_id": "customer-1", "restaurant_id": "restaurant-1", "items": []map[string]any{{"dish_id": "pizza-margherita", "quantity": 2}}, "payment_token": token}
}
func (c *client) create(token string, want int, verbose bool) (s state, err error) {
	var data json.RawMessage
	code, h, err := c.request("POST", c.order+"/api/v1/orders", input(token), "", &data)
	if err != nil {
		return s, err
	}
	if code != want {
		return s, fmt.Errorf("checkout HTTP %d, expected %d: %s", code, want, data)
	}
	var o order
	if code == 402 {
		var failure struct{ Order order }
		if err = json.Unmarshal(data, &failure); err != nil {
			return s, err
		}
		o = failure.Order
	} else {
		if err = json.Unmarshal(data, &o); err != nil {
			return s, err
		}
	}
	s = state{OrderID: o.ID, PaymentID: o.PaymentID, TraceID: h.Get("X-Trace-ID"), Traceparent: h.Get("X-Workflow-Traceparent")}
	if s.OrderID == "" {
		return s, errors.New("missing order ID")
	}
	if verbose {
		fmt.Printf("POST /api/v1/orders => %d\n%s\norder_id=%s\ntrace_id=%s\ntraceparent=%s\n", code, data, s.OrderID, s.TraceID, s.Traceparent)
	}
	return s, nil
}
func (c *client) waitStatus(address, want string) error {
	deadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		var value struct{ Status string }
		code, _, err := c.request("GET", address, nil, "", &value)
		if err == nil && code == 200 && value.Status == want {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s at %s", want, address)
}
func (c *client) happy(verbose bool) (s state, err error) {
	s, err = c.create("tok-success", 201, verbose)
	if err != nil {
		return s, err
	}
	if err = c.waitStatus(c.fulfillment+"/api/v1/fulfillments/"+s.OrderID, "Accepted"); err != nil {
		return s, err
	}
	for _, want := range []string{"Cooking", "Ready", "Delivering", "Completed"} {
		var f fulfillment
		code, _, e := c.request("POST", c.fulfillment+"/api/v1/fulfillments/"+s.OrderID+"/advance", nil, s.Traceparent, &f)
		if e != nil {
			return s, e
		}
		if code != 200 || f.Status != want {
			return s, fmt.Errorf("advance: HTTP %d status=%s, expected %s", code, f.Status, want)
		}
		if e = c.waitStatus(c.order+"/api/v1/orders/"+s.OrderID, want); e != nil {
			return s, e
		}
		if verbose {
			fmt.Println("Fulfillment -> Kafka -> Order:", want)
		}
	}
	if err = c.checkRow("ORDER_DB_URL", `SELECT count(*) FROM orders WHERE id=$1 AND status='Completed' AND sequence=5`, s.OrderID, 1); err != nil {
		return s, err
	}
	if err = c.checkRow("PAYMENT_DB_URL", `SELECT count(*) FROM payments WHERE order_id=$1 AND status='Succeeded'`, s.OrderID, 1); err != nil {
		return s, err
	}
	if err = c.checkRow("FULFILLMENT_DB_URL", `SELECT count(*) FROM fulfillments WHERE order_id=$1 AND status='Completed' AND sequence=5`, s.OrderID, 1); err != nil {
		return s, err
	}
	paid, e := c.messages(events.PaidTopic)
	if e != nil {
		return s, e
	}
	if countKey(paid, s.OrderID) < 1 {
		return s, errors.New("paid event missing")
	}
	changed, e := c.messages(events.StatusTopic)
	if e != nil {
		return s, e
	}
	seen := map[int]bool{}
	for _, m := range changed {
		if string(m.Key) == s.OrderID {
			var event events.StatusChanged
			if err = json.Unmarshal(m.Value, &event); err != nil {
				return s, err
			}
			seen[event.Sequence] = true
		}
	}
	if len(seen) != 5 {
		return s, fmt.Errorf("expected 5 distinct status sequences, got %v", seen)
	}
	return s, nil
}
func (c *client) decline(s *state, verbose bool) error {
	f, err := c.create("tok-decline", 402, verbose)
	if err != nil {
		return err
	}
	s.FailedOrderID = f.OrderID
	s.FailedTraceID = f.TraceID
	if err = c.verifyFailure(f.OrderID); err != nil {
		return err
	}
	if verbose {
		fmt.Println("SUCCESS: Declined, no paid event/outbox/fulfillment")
	}
	return nil
}
func (c *client) verifyFailure(id string) error {
	if id == "" {
		return errors.New("no failed order; run make failure")
	}
	if err := c.checkRow("ORDER_DB_URL", `SELECT count(*) FROM orders WHERE id=$1 AND status='PaymentDeclined'`, id, 1); err != nil {
		return err
	}
	if err := c.checkRow("PAYMENT_DB_URL", `SELECT count(*) FROM payments WHERE order_id=$1 AND status='Declined'`, id, 1); err != nil {
		return err
	}
	if err := c.checkRow("ORDER_DB_URL", `SELECT count(*) FROM outbox WHERE message_key=$1`, id, 0); err != nil {
		return err
	}
	if err := c.checkRow("FULFILLMENT_DB_URL", `SELECT count(*) FROM fulfillments WHERE order_id=$1`, id, 0); err != nil {
		return err
	}
	var value json.RawMessage
	code, _, err := c.request("GET", c.fulfillment+"/api/v1/fulfillments/"+id, nil, "", &value)
	if err != nil {
		return err
	}
	if code != 404 {
		return fmt.Errorf("failed order fulfillment HTTP %d", code)
	}
	messages, err := c.messages(events.PaidTopic)
	if err != nil {
		return err
	}
	if countKey(messages, id) != 0 {
		return errors.New("declined order unexpectedly published a paid event")
	}
	return nil
}
func (c *client) checkRow(key, query, id string, want int) error {
	address := config.Env(key, "")
	if address == "" {
		return fmt.Errorf("%s is required", key)
	}
	db, err := sql.Open("pgx", address)
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var count int
	if err = db.QueryRowContext(ctx, query, id).Scan(&count); err != nil {
		return err
	}
	if count != want {
		return fmt.Errorf("%s: got %d rows, expected %d for %s", key, count, want, id)
	}
	return nil
}
func countKey(messages []kafka.Message, key string) int {
	n := 0
	for _, m := range messages {
		if string(m.Key) == key {
			n++
		}
	}
	return n
}

// Читаем конечный snapshot всех partitions. Это независимый observer без group;
// он не меняет offsets бизнес-consumers и не ждёт бесконечно новых событий.
func (c *client) messages(topic string) (all []kafka.Message, err error) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	conn, err := kafka.DialContext(ctx, "tcp", c.broker)
	if err != nil {
		return nil, err
	}
	parts, err := conn.ReadPartitions(topic)
	conn.Close()
	if err != nil {
		return nil, err
	}
	for _, p := range parts {
		leader, e := kafka.DialLeader(ctx, "tcp", c.broker, topic, p.ID)
		if e != nil {
			return nil, e
		}
		first, e := leader.ReadFirstOffset()
		if e != nil {
			leader.Close()
			return nil, e
		}
		end, e := leader.ReadLastOffset()
		leader.Close()
		if e != nil {
			return nil, e
		}
		if first >= end {
			continue
		}
		reader := kafka.NewReader(kafka.ReaderConfig{Brokers: []string{c.broker}, Topic: topic, Partition: p.ID, MinBytes: 1, MaxBytes: 1e6, MaxWait: 200 * time.Millisecond})
		if e = reader.SetOffset(first); e != nil {
			reader.Close()
			return nil, e
		}
		for {
			m, e := reader.FetchMessage(ctx)
			if e != nil {
				reader.Close()
				return nil, e
			}
			if m.Offset >= end {
				break
			}
			all = append(all, m)
			if m.Offset == end-1 {
				break
			}
		}
		reader.Close()
	}
	return all, nil
}
func (c *client) inspect() error {
	for _, topic := range []string{events.PaidTopic, events.StatusTopic} {
		m, err := c.messages(topic)
		if err != nil {
			return err
		}
		fmt.Printf("\nTOPIC %s, snapshot messages=%d\n", topic, len(m))
		for _, e := range m {
			h := map[string]string{}
			for _, v := range e.Headers {
				h[v.Key] = string(v.Value)
			}
			fmt.Printf("partition=%d offset=%d key=%s headers=%v\npayload=%s\n", e.Partition, e.Offset, e.Key, h, e.Value)
		}
	}
	return nil
}
func (c *client) health() error {
	checks := map[string]string{"order": c.order + "/health", "payment": c.payment + "/health", "fulfillment": c.fulfillment + "/health", "collector": config.Env("OTEL_HEALTH_URL", "http://localhost:13133/"), "prometheus": config.Env("PROMETHEUS_URL", "http://localhost:9090") + "/-/ready", "grafana": config.Env("GRAFANA_URL", "http://localhost:3000") + "/api/health", "loki": config.Env("LOKI_URL", "http://localhost:3100") + "/ready", "jaeger": config.Env("JAEGER_URL", "http://localhost:16686") + "/api/services"}
	for name, address := range checks {
		code, _, err := c.request("GET", address, nil, "", nil)
		if err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
		if code != 200 {
			return fmt.Errorf("%s HTTP %d", name, code)
		}
		fmt.Println(name, "OK")
	}
	for _, key := range []string{"ORDER_DB_URL", "PAYMENT_DB_URL", "FULFILLMENT_DB_URL"} {
		db, err := sql.Open("pgx", config.Env(key, ""))
		if err != nil {
			return err
		}
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		err = db.PingContext(ctx)
		cancel()
		db.Close()
		if err != nil {
			return err
		}
		fmt.Println(key, "OK")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, err := kafka.DialContext(ctx, "tcp", c.broker)
	if err != nil {
		return err
	}
	defer conn.Close()
	p, err := conn.ReadPartitions(events.PaidTopic, events.StatusTopic)
	if err != nil {
		return err
	}
	if len(p) < 2 {
		return errors.New("Kafka topics are missing")
	}
	fmt.Println("kafka OK")
	return nil
}
func (c *client) observe(s state) error {
	deadline := time.Now().Add(45 * time.Second)
	var last error
	for time.Now().Before(deadline) {
		if last = c.checkTelemetry(s); last == nil {
			fmt.Printf("SUCCESS: logs, metrics, DB/Kafka spans and runtime edges found\nhttp://localhost:16686/trace/%s\nhttp://localhost:3000/d/godelivery-technical\nhttp://localhost:3000/d/godelivery-business\nhttp://localhost:3000/d/godelivery-traffic\n", s.TraceID)
			return nil
		}
		time.Sleep(time.Second)
	}
	return last
}
func (c *client) checkTelemetry(s state) error {
	prom := config.Env("PROMETHEUS_URL", "http://localhost:9090")
	queries := []string{"sum(orders_created_total)", "sum(orders_paid_total)", "sum(orders_completed_total)", "sum(orders_failed_total)", "sum(payments_failed_total)", "sum(fulfillments_completed_total)"}
	for _, q := range queries {
		var v struct {
			Status string
			Data   struct {
				Result []struct{ Value []json.RawMessage }
			}
		}
		code, _, err := c.request("GET", prom+"/api/v1/query?query="+url.QueryEscape(q), nil, "", &v)
		if err != nil {
			return err
		}
		if code != 200 || v.Status != "success" || len(v.Data.Result) == 0 {
			return fmt.Errorf("no Prometheus data for %s", q)
		}
		var number string
		if len(v.Data.Result[0].Value) != 2 || json.Unmarshal(v.Data.Result[0].Value[1], &number) != nil {
			return fmt.Errorf("invalid Prometheus sample for %s", q)
		}
		n, e := strconv.ParseFloat(number, 64)
		if e != nil || n <= 0 {
			return fmt.Errorf("business counter did not increase: %s", q)
		}
	}
	var tr struct {
		Data []struct {
			Spans []struct {
				OperationName string
				Duration      int64
				Tags          []struct {
					Key string
				}
			}
			Processes map[string]struct{ ServiceName string }
		}
	}
	code, _, err := c.request("GET", config.Env("JAEGER_URL", "http://localhost:16686")+"/api/traces/"+s.TraceID, nil, "", &tr)
	if err != nil {
		return err
	}
	if code != 200 || len(tr.Data) == 0 {
		return errors.New("workflow trace not found")
	}
	services := map[string]bool{}
	for _, p := range tr.Data[0].Processes {
		services[p.ServiceName] = true
	}
	for _, name := range []string{"order-service", "payment-service", "fulfillment-service"} {
		if !services[name] {
			return fmt.Errorf("trace lacks %s", name)
		}
	}
	var db, publish, consume bool
	for _, span := range tr.Data[0].Spans {
		if strings.HasPrefix(span.OperationName, "SQL ") {
			tags := map[string]bool{}
			for _, a := range span.Tags {
				tags[a.Key] = true
			}
			db = db || (span.Duration > 0 && tags["db.system"] && tags["db.operation"] && tags["db.collection"])
		}
		publish = publish || strings.HasPrefix(span.OperationName, "Kafka publish")
		consume = consume || strings.HasPrefix(span.OperationName, "Kafka consume")
	}
	if !db || !publish || !consume {
		return errors.New("trace lacks SQL/producer/consumer spans")
	}
	loki := config.Env("LOKI_URL", "http://localhost:3100")
	query := `{service_name=~".+"} | json | trace_id="` + s.TraceID + `"`
	var logs struct {
		Data struct{ Result []json.RawMessage }
	}
	code, _, err = c.request("GET", loki+"/loki/api/v1/query_range?query="+url.QueryEscape(query)+"&limit=100", nil, "", &logs)
	if err != nil {
		return err
	}
	if code != 200 || len(logs.Data.Result) == 0 {
		return errors.New("workflow logs not found in Loki")
	}
	for _, uid := range []string{"godelivery-technical", "godelivery-business", "godelivery-traffic"} {
		code, _, err = c.request("GET", config.Env("GRAFANA_URL", "http://localhost:3000")+"/api/dashboards/uid/"+uid, nil, "", nil)
		if err != nil {
			return err
		}
		if code != 200 {
			return fmt.Errorf("dashboard %s HTTP %d", uid, code)
		}
	}
	// Проверяем данные именно того PromQL/frame, из которого Node Graph строит карту.
	// C4 source или наличие dashboard JSON не доказывают runtime-взаимодействия.
	graphQuery := `label_join(sum by(source,target)(increase(service_calls_total[15m])),"id"," -> ","source","target") > 0`
	var graph struct {
		Data struct {
			Result []struct {
				Metric map[string]string
				Value  []json.RawMessage
			}
		}
	}
	code, _, err = c.request("GET", prom+"/api/v1/query?query="+url.QueryEscape(graphQuery), nil, "", &graph)
	if err != nil || code != 200 {
		return fmt.Errorf("runtime map query HTTP %d: %v", code, err)
	}
	edges := map[string]bool{}
	for _, row := range graph.Data.Result {
		if len(row.Value) != 2 {
			continue
		}
		var number string
		_ = json.Unmarshal(row.Value[1], &number)
		n, _ := strconv.ParseFloat(number, 64)
		if n > 0 {
			edges[row.Metric["id"]] = true
		}
	}
	for _, edge := range []string{"order-service -> payment-service", "order-service -> kafka", "kafka -> fulfillment-service", "fulfillment-service -> kafka", "kafka -> order-service"} {
		if !edges[edge] {
			return fmt.Errorf("runtime map lacks observed edge %s; run make traffic", edge)
		}
	}
	var frames struct {
		Results map[string]struct {
			Frames []struct {
				Schema struct {
					Fields []struct{ Name string }
				}
				Data struct{ Values [][]any }
			}
		}
	}
	input := map[string]any{
		"from": strconv.FormatInt(time.Now().Add(-15*time.Minute).UnixMilli(), 10),
		"to":   strconv.FormatInt(time.Now().UnixMilli(), 10),
		"queries": []any{map[string]any{"refId": "edges", "expr": graphQuery, "format": "table", "instant": true,
			"datasource": map[string]string{"uid": "prometheus", "type": "prometheus"}, "intervalMs": 5000, "maxDataPoints": 100}},
	}
	code, _, err = c.request("POST", config.Env("GRAFANA_URL", "http://localhost:3000")+"/api/ds/query", input, "", &frames)
	if err != nil || code != 200 || len(frames.Results["edges"].Frames) == 0 {
		return fmt.Errorf("Grafana runtime map datasource frame unavailable, HTTP %d: %v", code, err)
	}
	frame := frames.Results["edges"].Frames[0]
	fields := map[string]bool{}
	for _, field := range frame.Schema.Fields {
		fields[field.Name] = true
	}
	if !fields["id"] || !fields["source"] || !fields["target"] || len(frame.Data.Values) == 0 || len(frame.Data.Values[0]) == 0 {
		return errors.New("Grafana runtime map frame lacks id/source/target or rows")
	}
	return nil
}
