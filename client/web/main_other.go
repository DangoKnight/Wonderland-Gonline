//go:build !(js && wasm)

package main

// The browser client only builds for GOOS=js GOARCH=wasm.
func main() {
	println("client/web builds for GOOS=js GOARCH=wasm only")
}
