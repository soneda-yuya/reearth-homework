package mofa

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/soneda-yuya/reearth-homework/internal/platform/observability"
	"github.com/soneda-yuya/reearth-homework/internal/platform/retry"
	"github.com/soneda-yuya/reearth-homework/internal/safetyincident/domain"
	"github.com/soneda-yuya/reearth-homework/internal/shared/errs"
)

// Source is the MOFA OpenData adapter. It picks the right URL for the
// requested mode, fetches the XML with retry, and decodes it into MailItems.
type Source struct {
	baseURL string
	http    *http.Client
}

// New returns a Source backed by the given HTTP client. Callers typically
// pass a client with a timeout already configured.
func New(baseURL string, httpClient *http.Client) *Source {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	return &Source{baseURL: baseURL, http: httpClient}
}

// Fetch returns every item published at the given mode's MOFA endpoint.
// Items whose LeaveDate cannot be parsed are dropped silently and counted
// via the `mofa.items.dropped` span attribute — they are not worth failing
// the whole Run for, but the count is visible in traces for triage.
func (s *Source) Fetch(ctx context.Context, mode domain.IngestionMode) ([]domain.MailItem, error) {
	url, err := s.urlFor(mode)
	if err != nil {
		return nil, err
	}

	ctx, span := observability.Tracer(ctx).Start(ctx, "mofa.Fetch",
		trace.WithAttributes(
			attribute.String("mode", string(mode)),
			attribute.String("url", url),
		))
	defer span.End()

	body, err := s.fetchBody(ctx, url)
	if err != nil {
		return nil, err
	}

	var feed mofaFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, errs.Wrap("mofa.decode", errs.KindExternal, err)
	}

	items := make([]domain.MailItem, 0, len(feed.Items))
	dropped := 0
	for _, raw := range feed.Items {
		item, ok := convert(raw)
		if !ok {
			dropped++
			continue
		}
		items = append(items, item)
	}
	if dropped > 0 {
		span.SetAttributes(attribute.Int("mofa.items.dropped", dropped))
	}
	span.SetAttributes(attribute.Int("mofa.items.parsed", len(items)))

	return items, nil
}

func (s *Source) urlFor(mode domain.IngestionMode) (string, error) {
	switch mode {
	case domain.IngestionModeIncremental:
		return s.baseURL + "/newarrivalA.xml", nil
	case domain.IngestionModeInitial:
		return s.baseURL + "/00A.xml", nil
	default:
		return "", errs.Wrap("mofa.url",
			errs.KindInvalidInput,
			fmt.Errorf("unknown mode %q", mode))
	}
}

// fetchBody issues a GET with retry on 5xx/429. Body reads happen inside the
// retry loop because a partial transfer is also transient.
func (s *Source) fetchBody(ctx context.Context, url string) ([]byte, error) {
	var body []byte
	err := retry.Do(ctx, retry.DefaultPolicy, func(ctx context.Context) error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			return errs.Wrap("mofa.new_request", errs.KindInternal, err)
		}
		req.Header.Set("Accept", "application/xml, text/xml")

		resp, err := s.http.Do(req)
		if err != nil {
			return errs.Wrap("mofa.http", errs.KindExternal, err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			return errs.Wrap("mofa.transient", errs.KindExternal,
				fmt.Errorf("HTTP %d", resp.StatusCode))
		}
		if resp.StatusCode != http.StatusOK {
			return errs.Wrap("mofa.http_status", errs.KindInternal,
				fmt.Errorf("HTTP %d", resp.StatusCode))
		}

		b, err := io.ReadAll(resp.Body)
		if err != nil {
			return errs.Wrap("mofa.read_body", errs.KindExternal, err)
		}
		body = b
		return nil
	})
	return body, err
}

// convert maps a wire item to the domain shape, applying LeaveDate parsing.
// A row with an unparseable timestamp is dropped (returns false).
func convert(raw rawItem) (domain.MailItem, bool) {
	t, ok := parseLeaveDate(raw.LeaveDate)
	if !ok {
		return domain.MailItem{}, false
	}
	return domain.MailItem{
		KeyCd:       raw.KeyCd,
		InfoType:    raw.InfoType,
		InfoName:    raw.InfoName,
		LeaveDate:   t,
		Title:       raw.Title,
		Lead:        raw.Lead,
		MainText:    raw.MainText,
		InfoURL:     raw.InfoURL,
		KoukanCd:    raw.KoukanCd,
		KoukanName:  raw.KoukanName,
		AreaCd:      raw.AreaCd,
		AreaName:    raw.AreaName,
		CountryCd:   raw.CountryCd,
		CountryName: raw.CountryName,
	}, true
}
