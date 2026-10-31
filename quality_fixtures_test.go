package gosimilarity

// Original, parseable Go snippets. External names provide realistic context;
// each side is normalized independently and is not a standalone package.
var qualityFixtures = []struct {
	name, rationale string
	useful          bool
	a, b            string
}{
	{
		name:      "decode-config",
		rationale: "Read, decode, and wrap errors can share a loader parameterized by decoder and diagnostic context.",
		useful:    true,
		a: `func loadJSON(path string, dst any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read JSON config: %w", err)
	}
	if err := json.Unmarshal(data, dst); err != nil {
		return fmt.Errorf("decode JSON config: %w", err)
	}
	return nil
}`,
		b: `func loadYAML(path string, dst any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read YAML config: %w", err)
	}
	if err := yaml.Unmarshal(data, dst); err != nil {
		return fmt.Errorf("decode YAML config: %w", err)
	}
	return nil
}`,
	},
	{
		name:      "trim-nonempty-lines",
		rationale: "The same trim-and-filter operation is expressed with a guard versus a nested append.",
		useful:    true,
		a: `func cleanLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
	}
	return out
}`,
		b: `func nonemptyLines(lines []string) []string {
	var out []string
	for _, line := range lines {
		if cleaned := strings.TrimSpace(line); cleaned != "" {
			out = append(out, cleaned)
		}
	}
	return out
}`,
	},
	{
		name:      "sorted-map-keys",
		rationale: "Both collect and sort map keys; a generic helper can replace the different value types and allocation styles.",
		useful:    true,
		a: `func sortedUserNames(users map[string]User) []string {
	names := make([]string, 0, len(users))
	for name := range users {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}`,
		b: `func sortedGroupNames(groups map[string]Group) []string {
	var names []string
	for name := range groups {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}`,
	},
	{
		name:      "retry-operation",
		rationale: "Bounded retries with delay can share a helper taking the operation and retry policy.",
		useful:    true,
		a: `func sendWithRetry(ctx context.Context, attempts int) error {
	var err error
	for i := 0; i < attempts; i++ {
		err = send(ctx)
		if err == nil {
			return nil
		}
		time.Sleep(time.Second)
	}
	return err
}`,
		b: `func syncWithRetry(ctx context.Context, attempts int) error {
	var err error
	for i := 0; i < attempts; i++ {
		err = syncRemote(ctx)
		if err == nil {
			return nil
		}
		if !temporary(err) {
			return err
		}
		time.Sleep(2 * time.Second)
	}
	return err
}`,
	},
	{
		name:      "opposite-access-policies",
		rationale: "Owner-only access and owner exclusion encode opposite policies; common guard syntax alone is not a useful extraction.",
		useful:    false,
		a: `func allowOwner(user User, doc Document) error {
	if user.ID != doc.OwnerID {
		return ErrForbidden
	}
	return nil
}`,
		b: `func allowIndependentReviewer(user User, doc Document) error {
	if user.ID == doc.OwnerID {
		return ErrForbidden
	}
	return nil
}`,
	},
	{
		name:      "reserve-versus-release-stock",
		rationale: "Matching validation boilerplate surrounds inverse inventory transitions; merging these domain operations would obscure intent.",
		useful:    false,
		a: `func reserveStock(stock *Stock, quantity int) error {
	if quantity <= 0 {
		return ErrInvalidQuantity
	}
	if stock.Available < quantity {
		return ErrInsufficientStock
	}
	stock.Available -= quantity
	stock.Reserved += quantity
	return nil
}`,
		b: `func releaseStock(stock *Stock, quantity int) error {
	if quantity <= 0 {
		return ErrInvalidQuantity
	}
	if stock.Reserved < quantity {
		return ErrInsufficientStock
	}
	stock.Available += quantity
	stock.Reserved -= quantity
	return nil
}`,
	},
	{
		name:      "sum-versus-maximum",
		rationale: "A sum and a maximum solve different aggregations; a generic fold would add indirection to these short functions.",
		useful:    false,
		a: `func totalBytes(sizes []int64) int64 {
	var total int64
	for _, size := range sizes {
		total += size
	}
	return total
}`,
		b: `func largestFile(sizes []int64) int64 {
	var largest int64
	for _, size := range sizes {
		if size > largest {
			largest = size
		}
	}
	return largest
}`,
	},
	{
		name:      "csv-stream-versus-cache-expiry",
		rationale: "Streaming CSV output and synchronized cache expiry have unrelated data flow, lifetimes, and responsibilities.",
		useful:    false,
		a: `func writeRows(w io.Writer, rows [][]string) error {
	writer := csv.NewWriter(w)
	for _, row := range rows {
		if err := writer.Write(row); err != nil {
			return err
		}
	}
	writer.Flush()
	return writer.Error()
}`,
		b: `func (c *Cache) expire(now time.Time) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	removed := 0
	for key, entry := range c.entries {
		if now.After(entry.expiresAt) {
			delete(c.entries, key)
			removed++
		}
	}
	return removed
}`,
	},
}
