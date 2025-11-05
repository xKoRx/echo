// Package handshake contiene estructuras y utilidades para el protocolo de
// handshake entre los Expert Advisors (EAs), el Agent y el Core.
//
// Se encarga de:
//   - Normalizar payloads JSON provenientes de los EAs.
//   - Validar versión de protocolo y compatibilidad de features.
//   - Compartir estructuras reutilizables (Handshakes, rangos de versión, issue codes).
//
// El paquete está diseñado para ser consumido por Agent y Core, manteniendo la
// lógica de validación centralizada en el SDK.
package handshake
