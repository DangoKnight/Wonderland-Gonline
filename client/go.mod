module wonderland-gonline/client

go 1.25.0

require (
	github.com/hajimehoshi/ebiten/v2 v2.10.0
	golang.org/x/image v0.45.0
	wonderland-gonline v0.0.0
)

require (
	github.com/ebitengine/gomobile v0.0.0-20260820040257-d11f821a26a6 // indirect
	github.com/ebitengine/hideconsole v1.0.0 // indirect
	github.com/ebitengine/oto/v3 v3.5.0 // indirect
	github.com/ebitengine/purego v0.11.0 // indirect
	github.com/jfreymuth/pulse v0.1.3 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.41.0 // indirect
)

replace wonderland-gonline => ..

replace github.com/hajimehoshi/ebiten/v2 => ./third_party/ebiten
