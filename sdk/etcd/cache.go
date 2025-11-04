package etcd

import (
	"context"
	"fmt"
	"math"
	"sync/atomic"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// WatchEvent representa un evento de cambio en ETCD
type WatchEvent struct {
	Key   string
	Value string
	Type  WatchEventType
}

// WatchEventType define los tipos de eventos de watch
type WatchEventType int

const (
	WatchEventPut WatchEventType = iota
	WatchEventDelete
)

// CacheClient define una interfaz para operaciones de caché con capacidades de lectura y watch
type CacheClient interface {
	// Get obtiene un valor de la caché por su clave
	Get(key string) (string, bool)
	// SetVar establece un valor en ETCD y actualiza la caché local
	SetVar(ctx context.Context, key, val string) error
	// Close libera los recursos utilizados por la caché
	Close() error
	// Reload fuerza una recarga de la caché desde etcd
	Reload() error
	// WatchKey observa cambios en una clave específica
	WatchKey(ctx context.Context, key string) (<-chan WatchEvent, error)
	// WatchPrefix observa cambios en todas las claves con un prefijo específico
	WatchPrefix(ctx context.Context, prefix string) (<-chan WatchEvent, error)
}

// Cache implementa CacheClient utilizando un store interno basado en atomic.Value
type Cache struct {
	cli     *Client
	store   atomic.Value // map[string]string
	closeCh chan struct{}
	// opciones
	periodicRefresh time.Duration
	backoffBase     time.Duration
	backoffMax      time.Duration
}

// NewCache carga las claves bajo `prefix` y se suscribe a cambios.
// Devuelve un cliente de caché que mantiene sincronizada una copia local de los datos
// y escucha actualizaciones en tiempo real.
func NewCache(cli *Client) (CacheClient, error) {
	c := &Cache{
		cli:             cli,
		closeCh:         make(chan struct{}),
		periodicRefresh: 15 * time.Second,
		backoffBase:     250 * time.Millisecond,
		backoffMax:      30 * time.Second,
	}

	if err := c.Reload(); err != nil {
		return nil, err
	}

	//go c.watchLoop()    // hot‑reload con reintentos
	//go c.periodicLoop() // refresh periódico de seguridad
	return c, nil
}

// Get obtiene un valor de la caché por su clave
func (c *Cache) Get(key string) (string, bool) {
	val, err := c.cli.GetVar(context.Background(), key)
	if err != nil {
		return "", false
	}
	return val, true

	/*m, _ := c.store.Load().(map[string]string)
	// Lookup directo
	if val, ok := m[key]; ok {
		return val, true
	}
	// Lookup con prefijo de namespace
	ns := c.cli.NamespacePrefix()
	if !strings.HasPrefix(key, ns) {
		fullKey := ns + key
		if val, ok := m[fullKey]; ok {
			return val, true
		}
	}
	return "", false*/
}

// Reload fuerza una recarga de la caché desde etcd
func (c *Cache) Reload() error {
	return c.reload()
}

// Close libera los recursos utilizados por la caché
func (c *Cache) Close() error {
	close(c.closeCh)
	return nil
}

// reload carga los datos de etcd y actualiza la caché interna
func (c *Cache) reload() error {
	ctx, cancel := context.WithTimeout(context.Background(), c.cli.timeout)
	defer cancel()

	resp, err := c.cli.kv.Get(ctx, c.cli.NamespacePrefix(), clientv3.WithPrefix())
	if err != nil {
		return fmt.Errorf("error reloading cache from etcd: %w", err)
	}
	fmt.Printf("RESPUESTAAAA: %v\n", resp)

	tmp := make(map[string]string, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		tmp[string(c.cli.NamespacePrefix())+string(kv.Key)] = string(kv.Value)
	}

	c.store.Store(tmp)
	return nil
}

// watchLoop mantiene un watcher vivo con backoff exponencial ante cancelaciones.
func (c *Cache) watchLoop() {
	attempt := 0
	for {
		ctx, cancel := context.WithCancel(context.Background())

		// Coordinación de cierre
		done := make(chan struct{})
		go func() { <-c.closeCh; cancel(); close(done) }()

		// Watch sobre namespace + subprefijo
		wch := c.cli.raw.Watch(ctx, c.cli.NamespacePrefix(), clientv3.WithPrefix())
		for w := range wch {
			if w.Canceled {
				break
			}
			if len(w.Events) > 0 {
				_ = c.reload()
			}
		}

		// Cierre solicitado
		select {
		case <-done:
			return
		default:
		}

		// Backoff exponencial
		attempt++
		time.Sleep(c.backoffDuration(attempt))
	}
}

func (c *Cache) periodicLoop() {
	if c.periodicRefresh <= 0 {
		return
	}
	t := time.NewTicker(c.periodicRefresh)
	defer t.Stop()
	for {
		select {
		case <-c.closeCh:
			return
		case <-t.C:
			_ = c.reload()
		}
	}
}

func (c *Cache) backoffDuration(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	exp := float64(c.backoffBase) * math.Pow(2, float64(attempt-1))
	d := time.Duration(exp)
	if d > c.backoffMax {
		d = c.backoffMax
	}
	return d
}

// WatchKey observa cambios en una clave específica
func (c *Cache) WatchKey(ctx context.Context, key string) (<-chan WatchEvent, error) {
	return c.watch(ctx, key, false)
}

// WatchPrefix observa cambios en todas las claves con un prefijo específico
func (c *Cache) WatchPrefix(ctx context.Context, prefix string) (<-chan WatchEvent, error) {
	return c.watch(ctx, prefix, true)
}

// watch implementa la lógica común de observación para claves y prefijos
func (c *Cache) watch(ctx context.Context, target string, isPrefix bool) (<-chan WatchEvent, error) {
	eventCh := make(chan WatchEvent, 10) // buffer para evitar bloqueos

	go func() {
		defer close(eventCh)

		attempt := 0
		for {
			// Crear contexto cancelable para este intento de watch
			watchCtx, cancel := context.WithCancel(ctx)

			// Construir la clave completa con namespace
			fullKey := c.cli.NamespacePrefix() + target

			// Configurar opciones de watch
			var opts []clientv3.OpOption
			if isPrefix {
				opts = append(opts, clientv3.WithPrefix())
			}

			// Iniciar watch
			wch := c.cli.raw.Watch(watchCtx, fullKey, opts...)

			// Procesar eventos
			for watchResp := range wch {
				if watchResp.Canceled {
					cancel()
					break
				}

				// Procesar eventos y enviarlos al canal
				for _, event := range watchResp.Events {
					watchEvent := WatchEvent{
						Key:   string(event.Kv.Key),
						Value: string(event.Kv.Value),
					}

					switch event.Type {
					case clientv3.EventTypePut:
						watchEvent.Type = WatchEventPut
					case clientv3.EventTypeDelete:
						watchEvent.Type = WatchEventDelete
					}

					select {
					case eventCh <- watchEvent:
					case <-ctx.Done():
						cancel()
						return
					}
				}
			}

			cancel()

			// Verificar si el contexto padre fue cancelado
			select {
			case <-ctx.Done():
				return
			default:
			}

			// Backoff exponencial antes de reintentar
			attempt++
			backoffTime := c.backoffDuration(attempt)
			timer := time.NewTimer(backoffTime)

			select {
			case <-timer.C:
				// Continuar con el siguiente intento
			case <-ctx.Done():
				timer.Stop()
				return
			}
		}
	}()

	return eventCh, nil
}

// SetVar establece un valor en ETCD y actualiza la caché local
func (c *Cache) SetVar(ctx context.Context, key, val string) error {
	return c.cli.SetVar(ctx, key, val)
	/*ctx, cancel := context.WithTimeout(ctx, c.cli.timeout)
	defer cancel()

	// Asegurar prefijo de namespace
	ns := c.cli.NamespacePrefix()
	fullKey := key
	if !strings.HasPrefix(key, ns) {
		fullKey = ns + key
	}

	// Primero actualizar en ETCD
	if err := c.cli.SetVar(ctx, fullKey, val); err != nil {
		return err
	}

	// Luego actualizar la caché local de forma atómica
	m, _ := c.store.Load().(map[string]string)
	newMap := make(map[string]string, len(m)+1)
	for k, v := range m {
		newMap[k] = v
	}
	newMap[fullKey] = val
	c.store.Store(newMap)

	return nil*/
}
