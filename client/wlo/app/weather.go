package app

import (
	"time"

	"wonderland-gonline/client/wlo/weather"
	"wonderland-gonline/client/wlo/world"
)

// weatherLayer is the weather object, made on first use. Its Random is
// seeded from the clock, as Randomize does.
func (c *Client) weatherLayer() *weather.Layer {
	if c.weather == nil {
		r := &weather.Rand{Seed: uint32(time.Now().UnixMilli())}
		c.weather = weather.New(c.Pics, r.Intn)
	}
	return c.weather
}

// attachWeather gives a map's view the layer and its scene's weather.
func (c *Client) attachWeather(w *world.World) {
	if c.sceneWeather == nil {
		c.resources.loadScenes(c.Assets)
		c.sceneWeather = c.resources.sceneWeather
	}
	w.Weather = c.weatherLayer()
	w.WeatherKind = weather.SceneKind(c.sceneWeather[c.mapScenes[w.Player.Map]])
}
