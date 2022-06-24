## Building

```
GOOS=js GOARCH=wasm go build -o main.wasm
```

## Tips/Tricks
https://zchee.github.io/golang-wiki/WebAssembly/

## Run Without The Browser

```
$ GOOS=js GOARCH=wasm go run -exec="$(go env GOROOT)/misc/wasm/go_js_wasm_exec" .
```

You can put CLI arguments after the `.` if you need to set them.

## Deploying

1. The server will need to support CORS headers.
2. The server that hosts the wasm will need to have `.wasm` files specify the proper content type (`application/wasm`)

## TODOs:

1. Fix the non-streaming behavior of the `Post` method's wasm implementation.
2. Let JJ make a UI.
