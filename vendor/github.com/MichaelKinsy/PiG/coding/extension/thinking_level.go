package extension

// ThinkingLevel returns the live thinking level from the callback captured when
// this context was created. An unbound callback returns an empty level. Inactive
// contexts return their lifetime error instead of invoking the callback.
func (c *Context) ThinkingLevel() (ThinkingLevel, error) {
	if err := c.assertActive(); err != nil {
		return "", err
	}
	if c.actions.GetThinkingLevel == nil {
		return "", nil
	}
	return c.actions.GetThinkingLevel(), nil
}
