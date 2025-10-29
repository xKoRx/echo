package etcd

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
)

// Utilidad de migración entre namespaces (service/env) de etcd.
// Sin variables de entorno: usa configuración local del test y clientes con defaults.
// Caso base: fullDump de demo/development -> demo/production y verificación de copia.

func TestEtcdDumpUtility(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Parámetros locales (sin entorno)
	srcApp := "sqx-flowkit"
	srcEnv := "development"
	dstApp := "sqx-flowkit"
	dstEnv := "production"
	subprefix := "" // opcional, ej: "config/"

	srcClient, err := New(
		WithApp(srcApp),
		WithEnv(srcEnv),
	)
	if err != nil {
		t.Fatalf("no se pudo crear cliente origen: %v", err)
	}
	defer srcClient.Close()

	dstClient, err := New(
		WithApp(dstApp),
		WithEnv(dstEnv),
	)
	if err != nil {
		t.Fatalf("no se pudo crear cliente destino: %v", err)
	}
	defer dstClient.Close()

	// Log pre copia
	srcMap, err := listAll(ctx, srcClient, subprefix)
	if err != nil {
		t.Skipf("etcd origen no accesible o sin datos: %v", err)
	}
	dstMap, err := listAll(ctx, dstClient, subprefix)
	if err != nil {
		t.Skipf("etcd destino no accesible o sin datos: %v", err)
	}
	t.Logf("Antes: Origen %s/%s=%d claves | Destino %s/%s=%d (prefix='%s')",
		srcApp, srcEnv, len(srcMap), dstApp, dstEnv, len(dstMap), subprefix)

	// Ejecutar solo fullDump
	n, err := fullDump(ctx, srcClient, dstClient, subprefix)
	if err != nil {
		t.Fatalf("fullDump error: %v", err)
	}
	t.Logf("fullDump: %d claves copiadas (sobre-escritas si existían)", n)

	// Validación: destino contiene todas las claves de origen con mismos valores
	verifyContains(t, ctx, srcClient, dstClient, subprefix)

	// Log post copia
	postDst, err := listAll(ctx, dstClient, subprefix)
	if err != nil {
		t.Fatalf("listAll destino post-copia falló: %v", err)
	}
	t.Logf("Después: Destino %s/%s=%d claves (prefix='%s')", dstApp, dstEnv, len(postDst), subprefix)
}

// fullDump: copia todas las claves del origen al destino, sobre-escribiendo existentes.
func fullDump(ctx context.Context, src, dst *Client, subprefix string) (int, error) {
	m, err := listAll(ctx, src, subprefix)
	if err != nil {
		return 0, err
	}
	count := 0
	for k, v := range m {
		if err := put(ctx, dst, k, v); err != nil {
			return count, fmt.Errorf("put destino %q: %w", k, err)
		}
		count++
	}
	return count, nil
}

// lightDump: copia solo claves que no existen en destino; no toca existentes.
func lightDump(ctx context.Context, src, dst *Client, subprefix string) (int, error) {
	srcMap, err := listAll(ctx, src, subprefix)
	if err != nil {
		return 0, err
	}
	dstMap, err := listAll(ctx, dst, subprefix)
	if err != nil {
		return 0, err
	}
	count := 0
	for k, v := range srcMap {
		if _, exists := dstMap[k]; exists {
			continue
		}
		if err := put(ctx, dst, k, v); err != nil {
			return count, fmt.Errorf("put destino %q: %w", k, err)
		}
		count++
	}
	return count, nil
}

// deleteAll: elimina todas las claves del destino bajo subprefix (relativo al namespace actual).
func deleteAll(ctx context.Context, cli *Client, subprefix string) (int64, error) {
	sub := normalizePrefix(subprefix)
	ctx2, cancel := context.WithTimeout(ctx, cli.timeout)
	defer cancel()
	resp, err := cli.kv.Delete(ctx2, sub, clientv3.WithPrefix())
	if err != nil {
		return 0, fmt.Errorf("delete prefix %q: %w", sub, err)
	}
	return resp.Deleted, nil
}

// listAll: devuelve todas las claves/valores bajo subprefix (relativo al namespace del cliente).
func listAll(ctx context.Context, cli *Client, subprefix string) (map[string]string, error) {
	sub := normalizePrefix(subprefix)
	ctx2, cancel := context.WithTimeout(ctx, cli.timeout)
	defer cancel()
	resp, err := cli.kv.Get(ctx2, sub, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("get prefix %q: %w", sub, err)
	}
	out := make(map[string]string, len(resp.Kvs))
	for _, kv := range resp.Kvs {
		out[string(kv.Key)] = string(kv.Value)
	}
	return out, nil
}

func put(ctx context.Context, cli *Client, key, val string) error {
	ctx2, cancel := context.WithTimeout(ctx, cli.timeout)
	defer cancel()
	if _, err := cli.kv.Put(ctx2, key, val); err != nil {
		return err
	}
	return nil
}

// equalMaps compara origen y destino post-operación y retorna un diff humano.
func equalMaps(ctx context.Context, src, dst *Client, subprefix string) (bool, string) {
	srcMap, err := listAll(ctx, src, subprefix)
	if err != nil {
		return false, fmt.Sprintf("error listAll src: %v", err)
	}
	dstMap, err := listAll(ctx, dst, subprefix)
	if err != nil {
		return false, fmt.Sprintf("error listAll dst: %v", err)
	}
	if len(srcMap) != len(dstMap) {
		rep := diffReport(srcMap, dstMap)
		return false, rep.String()
	}
	for k, v := range srcMap {
		if dv, ok := dstMap[k]; !ok || dv != v {
			rep := diffReport(srcMap, dstMap)
			return false, rep.String()
		}
	}
	return true, ""
}

// verifyContains: destino contiene como superconjunto a origen con mismos valores para claves comunes.
func verifyContains(t *testing.T, ctx context.Context, src, dst *Client, subprefix string) {
	srcMap, err := listAll(ctx, src, subprefix)
	if err != nil {
		t.Fatalf("verifyContains listAll src: %v", err)
	}
	dstMap, err := listAll(ctx, dst, subprefix)
	if err != nil {
		t.Fatalf("verifyContains listAll dst: %v", err)
	}
	for k, v := range srcMap {
		if dv, ok := dstMap[k]; !ok || dv != v {
			t.Fatalf("destino no contiene clave %q con valor esperado; got=%q", k, dv)
		}
	}
}

// -------- Helpers de diff y utilitarios --------

type diff struct {
	ToCreate []string // en src pero no en dst
	ToUpdate []string // en ambos pero con valor diferente
	ToDelete []string // en dst pero no en src
}

func (d diff) String() string {
	return fmt.Sprintf("create=%d update=%d delete=%d", len(d.ToCreate), len(d.ToUpdate), len(d.ToDelete))
}

func diffReport(src, dst map[string]string) diff {
	rep := diff{}
	for k, sv := range src {
		if dv, ok := dst[k]; !ok {
			rep.ToCreate = append(rep.ToCreate, k)
		} else if dv != sv {
			rep.ToUpdate = append(rep.ToUpdate, k)
		}
	}
	for k := range dst {
		if _, ok := src[k]; !ok {
			rep.ToDelete = append(rep.ToDelete, k)
		}
	}
	sort.Strings(rep.ToCreate)
	sort.Strings(rep.ToUpdate)
	sort.Strings(rep.ToDelete)
	return rep
}

func sample(keys []string, n int) []string {
	if len(keys) <= n {
		return keys
	}
	return keys[:n]
}

func normalizePrefix(p string) string {
	if p == "" {
		return ""
	}
	// Operamos en un KV namespaced, por lo que el prefijo debe ser relativo.
	return strings.TrimPrefix(p, "/")
}

// Mantener helpers "usados" para evitar advertencias del analizador estático
var (
	_ = lightDump
	_ = deleteAll
	_ = equalMaps
	_ = diff{}
	_ = (diff{}).String
	_ = diffReport
	_ = sample
)
