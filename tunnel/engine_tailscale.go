package tunnel

// SetTailscaleConfig configures the experimental tsnet outbound. Passing an
// empty string clears the configuration. It returns an empty string on
// success for gomobile callers, or a safe validation error otherwise.
func (e *Engine) SetTailscaleConfig(jsonConfig string) string {
	if jsonConfig == "" {
		e.mu.Lock()
		e.tailscaleConfig = nil
		e.mu.Unlock()
		return ""
	}

	config, err := ParseTailscaleConfig(jsonConfig)
	if err != nil {
		return err.Error()
	}

	e.mu.Lock()
	e.tailscaleConfig = config
	e.mu.Unlock()
	return ""
}
