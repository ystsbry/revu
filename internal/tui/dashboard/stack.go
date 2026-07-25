package dashboard

// screenStack is the dashboard's navigation stack. The last element is the
// active screen — the one that receives messages and gets rendered.
//
// Screens below the top are kept alive rather than rebuilt, so returning
// from a detail view restores the exact cursor position and scroll offset
// the user left behind.
type screenStack struct {
	screens []Screen
}

func (s *screenStack) push(sc Screen) {
	if sc == nil {
		return
	}
	s.screens = append(s.screens, sc)
}

// pop drops the active screen and returns it. It refuses to empty the
// stack: popping the root screen would leave nothing to render, so the
// caller gets ok=false and the stack is left untouched.
func (s *screenStack) pop() (Screen, bool) {
	if len(s.screens) <= 1 {
		return nil, false
	}
	top := s.screens[len(s.screens)-1]
	s.screens = s.screens[:len(s.screens)-1]
	return top, true
}

// top returns the active screen, or nil when the stack is empty.
func (s *screenStack) top() Screen {
	if len(s.screens) == 0 {
		return nil
	}
	return s.screens[len(s.screens)-1]
}

func (s *screenStack) depth() int { return len(s.screens) }

// replaceTop swaps the active screen in place. Used to write back the
// model bubbletea hands us from Update.
func (s *screenStack) replaceTop(sc Screen) {
	if len(s.screens) == 0 || sc == nil {
		return
	}
	s.screens[len(s.screens)-1] = sc
}

// titles returns every screen's label, root first, for the breadcrumb.
func (s *screenStack) titles() []string {
	out := make([]string, 0, len(s.screens))
	for _, sc := range s.screens {
		out = append(out, sc.Title())
	}
	return out
}
