package platform

type WebDialogKind uint8

const (
	WebDialogAlert WebDialogKind = iota
	WebDialogConfirm
	WebDialogPrompt
	WebDialogBeforeUnload
)

type WebDialogRequest struct {
	ID                        uint64
	Kind                      WebDialogKind
	URL, Message, DefaultText string
}

type WebDialogResponse struct {
	Accepted bool
	Text     string
}

type WebFileChooserRequest struct {
	ID                    uint64
	URL                   string
	Multiple, Directories bool
	Accept                []string
}

type WebInteractionHost interface {
	HasWebDialogHandler(SurfaceID) bool
	OnWebDialog(SurfaceID, WebDialogRequest, func(WebDialogResponse)) bool
	OnWebFileChooser(SurfaceID, WebFileChooserRequest, func([]string)) bool
	OnWebPopup(SurfaceID, NewWindowRequest, func(SurfaceID, SurfaceID, SurfaceOptions) (Surface, error)) bool
}

type WebDialogs interface {
	SetWebDialogs(bool)
}
