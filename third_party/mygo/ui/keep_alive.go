package ui

// StateRoot returns the nearest StateScope's root, or the window Root when
// there is no scope. It owns per-page Local values while Root keeps its window
// meaning for focus, input observation and window-wide overlays.
func StateRoot(c *Context) Element {
	raw := c.build()
	if raw == nil {
		return Element{}
	}
	if raw.parent != nil && raw.parent.stateRoot != nil {
		return wrapElement(raw.parent.stateRoot)
	}
	return wrapElement(raw.root)
}

// StateScope builds children of root with an independent Local owner. The
// scope follows children built later by List and portals built by Overlay.
func StateScope(c *Context, root Element, build func()) {
	raw := c.build()
	if raw == nil {
		return
	}
	n := root.nodeFor(raw.rt)
	if n == nil {
		return
	}
	n.stateRoot = n
	n.Children(build)
}

// KeepAlive gives a page a stable, scoped identity. Active pages build their
// children; inactive pages keep their existing Local values, text editing,
// scroll state and resources without laying out or rebuilding the children.
// Omitting the page from a later build releases those resources normally.
func KeepAlive(c *Context, key any, active bool, build func()) Element {
	raw := c.build()
	if raw == nil {
		return Element{}
	}
	page := coreColumn(raw).Key(key).Grow(1).MinWidth(0).MinHeight(0)
	page.flags |= flagPage
	page.stateRoot = page
	focus := Local(wrapElement(page), "keep-alive-focus", func() struct {
		id     uint64
		active bool
	} {
		return struct {
			id     uint64
			active bool
		}{}
	})
	if raw.rt.within(raw.rt.focused, page.id) {
		focus.id = raw.rt.focused
		if !active {
			raw.rt.focused = 0
		}
	}
	if active {
		page.Children(build)
		if !focus.active && raw.rt.focused == 0 {
			if s := raw.rt.states[focus.id]; s != nil && s.seen == raw.rt.frame && s.pass == raw.rt.pass {
				raw.rt.focused = focus.id
				raw.rt.blinkStart = raw.rt.now()
			}
		}
	} else {
		page.Hidden(true)
		if raw.rt.kept == nil {
			raw.rt.kept = map[uint64]bool{}
		}
		raw.rt.kept[page.id] = true
	}
	focus.active = active
	return wrapElement(page)
}
