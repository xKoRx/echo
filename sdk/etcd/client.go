package etcd

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/xKoRx/sdk/constants"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/namespace"
)

const (
	defaultTimeout = 5
	envHost        = "ETCD_HOST"
	envPort        = "ETCD_PORT"
	envTimeout     = "ETCD_TIMEOUT"
	envScope       = "ENV"
)

type (
	// KV define las operaciones básicas que nos interesan de etcd (facilita mocking).
	KV interface {
		// Get obtiene un valor de etcd por su clave
		Get(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error)
		// Put establece un valor en etcd para una clave
		Put(ctx context.Context, key, val string, opts ...clientv3.OpOption) (*clientv3.PutResponse, error)
		// Delete elimina una clave de etcd
		Delete(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.DeleteResponse, error)
	}

	// Client encapsula la funcionalidad del cliente etcd con namespace configurado
	Client struct {
		raw     *clientv3.Client // cliente real
		kv      KV               // namespaced KV
		app     string           // nombre de la aplicación
		env     string           // entorno (development, testing, production)
		timeout time.Duration    // timeout para operaciones
		watcher watchClient      // watcher inyectable para tests (por defecto: raw)
	}
)

// watchClient expone sólo Watch, para facilitar mocking en pruebas.
type watchClient interface {
	Watch(ctx context.Context, key string, opts ...clientv3.OpOption) clientv3.WatchChan
}

// ---------- Constructor ----------

// Option define una función que modifica la configuración del cliente
type Option func(*config)

// config contiene la configuración para el cliente etcd
type config struct {
	endpoints []string      // lista de endpoints de etcd
	timeout   time.Duration // timeout para operaciones
	app       string        // nombre de la aplicación
	env       string        // entorno (development, testing, production)
	prefix    string        // prefijo personalizado (opcional)
}

// defaultConfig crea una configuración por defecto basada en variables de entorno
func defaultConfig() *config {
	// Determinar timeout primero
	timeout := defaultTimeout
	if i, err := strconv.Atoi(os.Getenv(envTimeout)); err == nil {
		timeout = i
	}

	// 1. Intentar leer lista de endpoints del cluster usando constante ETCD_ENDPOINTS
	if eps := os.Getenv(constants.EtcdEndpoints); eps != "" {
		parts := strings.Split(eps, ",")
		var clean []string
		for _, p := range parts {
			if trimmed := strings.TrimSpace(p); trimmed != "" {
				clean = append(clean, trimmed)
			}
		}
		if len(clean) > 0 {
			return &config{
				endpoints: clean,
				timeout:   time.Duration(timeout) * time.Second,
				app:       "default",
				env:       firstNonEmpty(os.Getenv(envScope), "development"),
			}
		}
	}

	return &config{
		endpoints: []string{"http://192.168.31.250:2379", "http://192.168.31.251:2379", "http://192.168.31.252:2379", "http://192.168.31.253:2379", "http://192.168.31.254:2379"},
		timeout:   time.Duration(timeout) * time.Second,
		app:       "default",
		env:       firstNonEmpty(os.Getenv(envScope), "development"),
	}
}

// WithEndpoints establece los endpoints del servidor etcd
func WithEndpoints(eps ...string) Option { return func(c *config) { c.endpoints = eps } }

// WithTimeout establece el timeout para las operaciones del cliente
func WithTimeout(t time.Duration) Option { return func(c *config) { c.timeout = t } }

// WithApp establece el nombre de la aplicación para el namespace
func WithApp(name string) Option { return func(c *config) { c.app = name } }

// WithEnv establece el entorno para el namespace
func WithEnv(env string) Option { return func(c *config) { c.env = env } }

// WithPrefix establece un prefijo personalizado para el namespace
func WithPrefix(p string) Option { return func(c *config) { c.prefix = p } }

// EndpointsFromEnv extrae la lista de endpoints del clúster leyendo la variable ETCD_ENDPOINTS.
// Devuelve nil si la variable no está definida o está vacía.
func EndpointsFromEnv() []string {
	eps := os.Getenv(constants.EtcdEndpoints)
	if eps == "" {
		return nil
	}
	parts := strings.Split(eps, ",")
	var clean []string
	for _, p := range parts {
		if trimmed := strings.TrimSpace(p); trimmed != "" {
			clean = append(clean, trimmed)
		}
	}
	if len(clean) == 0 {
		return nil
	}
	return clean
}

// WithEndpointsFromEnv establece los endpoints usando la variable ETCD_ENDPOINTS, si existe.
func WithEndpointsFromEnv() Option {
	return func(c *config) {
		if eps := EndpointsFromEnv(); len(eps) > 0 {
			c.endpoints = eps
		}
	}
}

// New crea un nuevo cliente etcd con la configuración proporcionada
func New(opts ...Option) (*Client, error) {
	cfg := defaultConfig()
	for _, opt := range opts {
		opt(cfg)
	}

	cli, err := clientv3.New(clientv3.Config{
		Endpoints:   cfg.endpoints,
		DialTimeout: cfg.timeout,
	})
	if err != nil {
		return nil, fmt.Errorf("error creating etcd client: %w", err)
	}

	// prefijo: /APP/ENV/
	if cfg.prefix == "" {
		cfg.prefix = fmt.Sprintf("/%s/%s/", cfg.app, cfg.env)
	}
	kv := namespace.NewKV(cli, cfg.prefix)

	return &Client{
		raw:     cli,
		kv:      kv,
		app:     cfg.app,
		env:     cfg.env,
		timeout: cfg.timeout,
		watcher: cli,
	}, nil
}

// ---------- Operaciones de alto nivel ----------

// NamespacePrefix devuelve el prefijo absoluto configurado para el cliente,
// con formato "/<app>/<env>/".
func (c *Client) NamespacePrefix() string {
	return fmt.Sprintf("/%s/%s/", c.app, c.env)
}

// GetVar obtiene una variable usando el patrón de namespace configurado
func (c *Client) GetVar(ctx context.Context, key string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.kv.Get(ctx, key)
	if err != nil {
		return "", fmt.Errorf("failed to get key %s: %w", key, err)
	}
	if len(resp.Kvs) == 0 {
		return "", fmt.Errorf("key not found: %s", key)
	}

	value := string(resp.Kvs[0].Value)
	return value, nil
}

// GetVarWithDefault obtiene una variable o devuelve un valor por defecto si no existe
func (c *Client) GetVarWithDefault(ctx context.Context, key, defaultValue string) (string, error) {
	value, err := c.GetVar(ctx, key)
	if err != nil {
		return defaultValue, nil
	}
	return value, nil
}

// GetVarInt obtiene una variable como entero
func (c *Client) GetVarInt(ctx context.Context, key string) (int, error) {
	value, err := c.GetVar(ctx, key)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(value)
}

// GetVarIntWithDefault obtiene una variable como entero o devuelve un valor por defecto
func (c *Client) GetVarIntWithDefault(ctx context.Context, key string, defaultValue int) (int, error) {
	value, err := c.GetVarInt(ctx, key)
	if err != nil {
		return defaultValue, nil
	}
	return value, nil
}

// GetVarBool obtiene una variable como booleano
func (c *Client) GetVarBool(ctx context.Context, key string) (bool, error) {
	value, err := c.GetVar(ctx, key)
	if err != nil {
		return false, err
	}
	return strconv.ParseBool(value)
}

// GetVarBoolWithDefault obtiene una variable como booleano o devuelve un valor por defecto
func (c *Client) GetVarBoolWithDefault(ctx context.Context, key string, defaultValue bool) (bool, error) {
	value, err := c.GetVarBool(ctx, key)
	if err != nil {
		return defaultValue, nil
	}
	return value, nil
}

// GetVarDuration obtiene una variable como duración (en milisegundos)
func (c *Client) GetVarDuration(ctx context.Context, key string) (time.Duration, error) {
	value, err := c.GetVarInt(ctx, key)
	if err != nil {
		return 0, err
	}
	return time.Duration(value) * time.Millisecond, nil
}

// GetVarDurationWithDefault obtiene una variable como duración o devuelve un valor por defecto
func (c *Client) GetVarDurationWithDefault(ctx context.Context, key string, defaultValue time.Duration) (time.Duration, error) {
	value, err := c.GetVarDuration(ctx, key)
	if err != nil {
		return defaultValue, nil
	}
	return value, nil
}

// SetVar establece una variable usando el patrón de namespace configurado
func (c *Client) SetVar(ctx context.Context, key, val string) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	_, err := c.kv.Put(ctx, key, val)
	if err != nil {
		return fmt.Errorf("failed to set key %s: %w", key, err)
	}

	return nil
}

// DeleteVar elimina una variable usando el patrón de namespace configurado
func (c *Client) DeleteVar(ctx context.Context, key string) error {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	_, err := c.kv.Delete(ctx, key)
	if err != nil {
		return fmt.Errorf("failed to delete key %s: %w", key, err)
	}

	return nil
}

// Close cierra la conexión con etcd
func (c *Client) Close() error {
	if c.raw != nil {
		return c.raw.Close()
	}
	return nil
}

// GetRawClient retorna el cliente ETCD nativo para operaciones avanzadas como Watch
func (c *Client) GetRawClient() *clientv3.Client {
	return c.raw
}

// ---------- Singleton opcional ----------

var (
	once          sync.Once
	defaultClient *Client
	initErr       error
)

// InitDefault crea (una sola vez) el cliente global.
// Llama a esta función en main() y propaga el error.
func InitDefault(opts ...Option) error {
	once.Do(func() {
		defaultClient, initErr = New(opts...)
	})
	return initErr
}

// Default devuelve el cliente global configurado previamente.
// Panic si no se ha inicializado.
func Default() *Client {
	if defaultClient == nil {
		panic("etcd client not initialized; call InitDefault first")
	}
	return defaultClient
}

// firstNonEmpty devuelve el primer valor no vacío de la lista
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
