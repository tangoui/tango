package platform

// ReportActivity records input at native dispatch, before it is consumed by
// a menu, canvas or child view. Hosts that do not track activity ignore it.
func ReportActivity(h Host, id SurfaceID) {
	if sink, ok := h.(interface{ OnSurfaceActivity(SurfaceID) }); ok {
		sink.OnSurfaceActivity(id)
	}
}
