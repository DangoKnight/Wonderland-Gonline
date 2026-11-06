package login

import (
	"wonderland-gonline/client/wlo/seui"
	"wonderland-gonline/client/wlo/surface"
)

// Cached backgrounds follow the same per-file fallback as registered buttons.
// Refresh the image without rebuilding controls or changing entered credentials.
func bindSkinBackground(owner seui.Control, property string, a Assets, name string, target **surface.Surface) {
	loaded := ""
	owner.Base().BindLanguage("skin:"+property, func() {
		path := a.SkinPicture("Menu", "Skins", "White", name)
		if path == loaded && *target != nil {
			return
		}
		image, err := loadJPEG(path)
		if err != nil {
			return
		}
		if *target != nil {
			(*target).Close()
		}
		*target = image
		loaded = path
	})
}
