// Package theme loads base16 color schemes and renders them as CSS.
//
// The built-in schemes in schemes/ come from the tinted-theming/schemes
// project (https://github.com/tinted-theming/schemes), distributed under the
// MIT License; see schemes/LICENSE. Refresh them with go generate.
package theme

//go:generate go run ../../cmd/tools schemes -out schemes
