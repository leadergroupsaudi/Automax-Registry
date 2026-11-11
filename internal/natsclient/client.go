package natsclient

import (
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/nats-io/nats.go"
)

const (
	// Stream is the JetStream stream Registry creates on the shared cluster.
	// Core and Dispatch keep their own streams (Core uses INCIDENT_STATE).
	Stream = "REGISTRY"

	// ConsumerPrefix is required on every Registry queue group and durable consumer.
	// Dispatch uses DispatchPrefix on the same cluster so the two services never
	// join each other's groups.
	ConsumerPrefix = "registry-"
	DispatchPrefix = "dispatch-"
)

const registryStream = Stream

var consumerNamePattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// Client publishes registry events. A failed connect does not stop the API:
// Publish returns an error until the background loop reconnects.
type Client struct {
	url        string
	retryEvery time.Duration

	mu   sync.RWMutex
	nc   *nats.Conn
	js   nats.JetStreamContext
	stop chan struct{}
	once sync.Once

	hooks []func()
}

func Connect(url string) *Client {
	c := &Client{
		url:        url,
		retryEvery: 5 * time.Second,
		stop:       make(chan struct{}),
	}
	if !c.tryConnect() {
		log.Printf("registry: NATS is unreachable at %s — API will keep running and retry in the background", url)
	}
	go c.reconnectLoop()
	return c
}

// AfterConnect runs fn after each successful connect, including a reconnect
// that replaces the connection. If the client is already connected, fn runs now.
func (c *Client) AfterConnect(fn func()) {
	if c == nil || fn == nil {
		return
	}
	c.mu.Lock()
	c.hooks = append(c.hooks, fn)
	connected := c.nc != nil && c.nc.IsConnected()
	c.mu.Unlock()
	if connected {
		fn()
	}
}

func (c *Client) notifyConnected() {
	c.mu.RLock()
	hooks := append([]func(){}, c.hooks...)
	c.mu.RUnlock()
	for _, fn := range hooks {
		fn()
	}
}

func (c *Client) Connected() bool {
	if c == nil {
		return false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.nc != nil && c.nc.IsConnected()
}

// ConsumerGroup returns a Registry consumer name. A bare purpose such as
// "thing-events" becomes "registry-thing-events". Names that already use the
// Dispatch prefix are rejected so Registry cannot join a Dispatch group.
func ConsumerGroup(purpose string) (string, error) {
	purpose = strings.TrimSpace(purpose)
	if purpose == "" {
		return "", fmt.Errorf("nats consumer name is empty")
	}
	if strings.HasPrefix(purpose, DispatchPrefix) {
		return "", fmt.Errorf("nats consumer %q uses the dispatch prefix; registry consumers must start with %s", purpose, ConsumerPrefix)
	}
	name := purpose
	if !strings.HasPrefix(name, ConsumerPrefix) {
		name = ConsumerPrefix + name
	}
	if !consumerNamePattern.MatchString(name) {
		return "", fmt.Errorf("nats consumer %q must match %s<purpose> using lowercase letters, digits, and hyphens", purpose, ConsumerPrefix)
	}
	return name, nil
}

// Publish stores the message in JetStream, then falls back to core NATS when
// JetStream is unavailable. Core subscribers on the same subject still see a
// successful JetStream publish.
func (c *Client) Publish(subject string, data []byte) error {
	if c == nil {
		return fmt.Errorf("nats client is nil")
	}
	c.mu.RLock()
	nc := c.nc
	js := c.js
	c.mu.RUnlock()
	if nc == nil || !nc.IsConnected() {
		return fmt.Errorf("nats is not connected")
	}
	if js != nil {
		if _, err := js.Publish(subject, data); err != nil {
			log.Printf("registry: JetStream publish failed for %s, falling back to core NATS: %v", subject, err)
		} else {
			return nil
		}
	}
	if err := nc.Publish(subject, data); err != nil {
		return fmt.Errorf("nats publish: %w", err)
	}
	return nil
}

// PublishCore sends a fire-and-forget core NATS message. It is not stored.
func (c *Client) PublishCore(subject string, data []byte) error {
	nc, _, err := c.conn()
	if err != nil {
		return err
	}
	if err := nc.Publish(subject, data); err != nil {
		return fmt.Errorf("nats core publish: %w", err)
	}
	return nc.FlushTimeout(2 * time.Second)
}

// PublishJetStream stores the message on the REGISTRY stream.
func (c *Client) PublishJetStream(subject string, data []byte) error {
	nc, js, err := c.conn()
	if err != nil {
		return err
	}
	if js == nil {
		return fmt.Errorf("nats jetstream is unavailable")
	}
	if _, err := js.Publish(subject, data); err != nil {
		return fmt.Errorf("nats jetstream publish: %w", err)
	}
	return nc.FlushTimeout(2 * time.Second)
}

// SubscribeCore joins a core queue group. The group is forced onto the registry- prefix.
func (c *Client) SubscribeCore(subject, group string, handler nats.MsgHandler) (*nats.Subscription, error) {
	if handler == nil {
		return nil, fmt.Errorf("nats handler is nil")
	}
	name, err := ConsumerGroup(group)
	if err != nil {
		return nil, err
	}
	nc, _, err := c.conn()
	if err != nil {
		return nil, err
	}
	sub, err := nc.QueueSubscribe(subject, name, handler)
	if err != nil {
		return nil, fmt.Errorf("nats core subscribe: %w", err)
	}
	if err := nc.FlushTimeout(2 * time.Second); err != nil {
		_ = sub.Unsubscribe()
		return nil, fmt.Errorf("nats core subscribe flush: %w", err)
	}
	return sub, nil
}

// SubscribeJetStream starts a durable consumer on the REGISTRY stream.
// The durable name is forced onto the registry- prefix. The caller keeps the
// subscription and acknowledges each message.
func (c *Client) SubscribeJetStream(subject, durable string, handler nats.MsgHandler) (*nats.Subscription, error) {
	if handler == nil {
		return nil, fmt.Errorf("nats handler is nil")
	}
	name, err := ConsumerGroup(durable)
	if err != nil {
		return nil, err
	}
	nc, js, err := c.conn()
	if err != nil {
		return nil, err
	}
	if js == nil {
		return nil, fmt.Errorf("nats jetstream is unavailable")
	}
	sub, err := js.Subscribe(subject, handler,
		nats.Durable(name),
		nats.BindStream(registryStream),
		nats.ManualAck(),
		nats.DeliverNew(),
	)
	if err != nil {
		return nil, fmt.Errorf("nats jetstream subscribe: %w", err)
	}
	if err := nc.FlushTimeout(2 * time.Second); err != nil {
		_ = sub.Unsubscribe()
		return nil, fmt.Errorf("nats jetstream subscribe flush: %w", err)
	}
	return sub, nil
}

func (c *Client) conn() (*nats.Conn, nats.JetStreamContext, error) {
	if c == nil {
		return nil, nil, fmt.Errorf("nats client is nil")
	}
	c.mu.RLock()
	nc := c.nc
	js := c.js
	c.mu.RUnlock()
	if nc == nil || !nc.IsConnected() {
		return nil, nil, fmt.Errorf("nats is not connected")
	}
	return nc, js, nil
}

func (c *Client) Close() {
	if c == nil {
		return
	}
	c.once.Do(func() { close(c.stop) })
	c.mu.Lock()
	nc := c.nc
	c.nc = nil
	c.js = nil
	c.mu.Unlock()
	if nc != nil {
		nc.Close()
	}
}

func (c *Client) reconnectLoop() {
	ticker := time.NewTicker(c.retryEvery)
	defer ticker.Stop()
	for {
		select {
		case <-c.stop:
			return
		case <-ticker.C:
			c.mu.RLock()
			connected := c.nc != nil && c.nc.IsConnected()
			c.mu.RUnlock()
			if connected {
				continue
			}
			c.tryConnect()
		}
	}
}

func (c *Client) tryConnect() bool {
	nc, err := nats.Connect(c.url,
		nats.Timeout(2*time.Second),
		nats.MaxReconnects(-1),
		nats.ReconnectWait(c.retryEvery),
		nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
			log.Printf("registry: NATS disconnected: %v", err)
		}),
		nats.ReconnectHandler(func(nc *nats.Conn) {
			log.Printf("registry: NATS reconnected to %s", nc.ConnectedUrl())
		}),
	)
	if err != nil {
		return false
	}

	var js nats.JetStreamContext
	js, jsErr := nc.JetStream()
	if jsErr != nil {
		log.Printf("registry: NATS connected but JetStream is unavailable: %v", jsErr)
	} else {
		ensureRegistryStream(js)
	}

	c.mu.Lock()
	old := c.nc
	c.nc = nc
	c.js = js
	c.mu.Unlock()
	if old != nil {
		old.Close()
	}
	log.Printf("registry: NATS connected to %s", nc.ConnectedUrl())
	c.notifyConnected()
	return true
}

func ensureRegistryStream(js nats.JetStreamContext) {
	if _, err := js.StreamInfo(registryStream); err == nil {
		return
	}
	_, err := js.AddStream(&nats.StreamConfig{
		Name:     registryStream,
		Subjects: []string{"registry.thing.>"},
		Storage:  nats.FileStorage,
	})
	if err != nil && !errorsIsStreamExists(err) {
		log.Printf("registry: NATS JetStream stream %s was not created: %v", registryStream, err)
	}
}

func errorsIsStreamExists(err error) bool {
	return err == nats.ErrStreamNameAlreadyInUse
}
