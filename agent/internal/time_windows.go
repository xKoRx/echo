//go:build windows
// +build windows

package internal

import "github.com/xKoRx/echo/sdk/utils"

// nowUnixMilli retorna el timestamp actual en milisegundos.
func nowUnixMilli() int64 {
	return utils.NowUnixMilli()
}
