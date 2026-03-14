package provider

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/foxzi/llm-translate/internal/config"
)

const TimeFocusPrompt = `Analyze the temporal focus of the following news text. Determine whether it describes past events, present situation, or future predictions/forecasts. Respond ONLY in this exact format:
TIME_FOCUS: <past|present|future|mixed> (<confidence 0.0-1.0>)
IS_PREDICTION: <true|false>
INDICATORS: <comma-separated list of temporal indicators found>

Rules:
- past: describes events that already happened, historical analysis, retrospective
- present: describes current situation, ongoing events, breaking news
- future: describes predictions, forecasts, expectations, planned events
- mixed: contains significant elements of multiple time frames
- IS_PREDICTION is true ONLY when text contains explicit predictions, forecasts, or speculations about future outcomes
- IS_PREDICTION is false for scheduled events, announcements, or plans already confirmed
- INDICATORS: extract temporal markers from text (e.g. "yesterday", "will", "expected to", "last year", "next quarter")
- Round confidence to 1 decimal place

Example responses:
TIME_FOCUS: future (0.8)
IS_PREDICTION: true
INDICATORS: expected to, forecast, will likely, next quarter

TIME_FOCUS: past (0.9)
IS_PREDICTION: false
INDICATORS: last week, announced, reported, was

Text to analyze:`

const AdDetectPrompt = `Analyze whether the following text is advertising or promotional content. Respond ONLY in this exact format:
AD_TYPE: <none|direct|native|sponsored|pr> (<confidence 0.0-1.0>)
AD_MARKERS: <comma-separated list of advertising indicators found>

Types:
- none: genuine editorial/news content with no advertising intent
- direct: explicit advertising, product promotion, call-to-action, commercial offer
- native: advertising disguised as editorial content, paid placement that mimics news
- sponsored: clearly marked sponsored content, paid partnership, branded content
- pr: press release, corporate announcement promoting company/product without editorial value

Markers to detect:
- product_promotion, call_to_action, brand_emphasis, price_mention, discount_offer, affiliate_links, excessive_praise, no_criticism, corporate_language, marketing_claims, promotional_tone, sponsored_disclosure, paid_partnership, one_sided_coverage

Rules:
- Be strict: genuine news about companies is NOT advertising (e.g. earnings report, regulatory action)
- native ads often lack criticism and use promotional tone while pretending to be news
- pr content focuses on company achievements without balanced perspective
- Round confidence to 1 decimal place

Example responses:
AD_TYPE: native (0.8)
AD_MARKERS: product_promotion, excessive_praise, no_criticism, promotional_tone

AD_TYPE: none (0.9)
AD_MARKERS: none

Text to analyze:`

const SentimentPrompt = `Analyze the sentiment of the following text. Respond ONLY with a single line in format:
SENTIMENT: <positive|negative|neutral> (<score from -1.0 to 1.0>)

Rules:
- Round score to exactly 1 decimal place (e.g. 0.7, not 0.65 or 0.700)
- Score must be consistent with label: positive > 0.0, negative < 0.0, neutral around 0.0
- Analyze overall tone, not individual sentences
- Factual reporting with no clear bias = neutral (score near 0.0)

Text to analyze:`

const TagsPromptTemplate = `Extract the %d most important keywords/tags from the following text.
Respond ONLY with a single line in format:
TAGS: tag1, tag2, tag3, ...

Normalization rules:
- Tags MUST be in the same language as the analyzed text
- Use lowercase only
- Use nominative case / base form (именительный падеж): "экономика" not "экономики", "рынок труда" not "рынка труда"
- Use single words or short phrases (2-3 words max)
- No hashtags, no articles, no prepositions
- No duplicates
- Prefer nouns and noun phrases

Text to analyze:`

const ClassifyPrompt = `Classify the following text into categories. Respond ONLY in this exact format:
TOPICS: <comma-separated list from: politics, economics, technology, medicine, incidents>
SCOPE: <comma-separated list from: regional, international>
TYPE: <comma-separated list from: corporate, regulatory, macro>

Rules:
- Select one or more values for each category
- Use only the exact values listed above
- If category doesn't apply, use "none"

Text to classify:`

const EmotionsPrompt = `Analyze the emotional tone of the following text. Respond ONLY in this exact format:
EMOTIONS: <comma-separated list of detected emotions with scores>

Available emotions and format:
fear:<0.0-1.0>, anger:<0.0-1.0>, hope:<0.0-1.0>, uncertainty:<0.0-1.0>, optimism:<0.0-1.0>, panic:<0.0-1.0>

Rules:
- Include only emotions with score > 0.1
- Score represents intensity (0.0 = absent, 1.0 = very strong)
- Round all scores to exactly 1 decimal place (e.g. 0.3, not 0.28 or 0.300)
- List emotions in descending order by score
- Be conservative: factual news articles typically have low emotion scores
- Advertising/promotional text should have high optimism, not high fear/panic

Example response:
EMOTIONS: fear:0.8, uncertainty:0.6, panic:0.3

Text to analyze:`

const FactualityPrompt = `Analyze the factuality and speculativeness of the following text. Respond ONLY in this exact format:
FACTUALITY: <type> (<confidence 0.0-1.0>)
EVIDENCE: <comma-separated list of evidence types found>

Types (choose one):
- confirmed: verified facts with clear sources or official data
- rumors: unverified information, hearsay, "sources say"
- forecasts: predictions, projections, future expectations
- unsourced: claims without attribution or evidence

Evidence types to detect:
- official_source, statistics, quotes, documents, expert_opinion, anonymous_source, speculation, prediction

Example response:
FACTUALITY: rumors (0.7)
EVIDENCE: anonymous_source, speculation

Text to analyze:`

const ImpactPrompt = `Analyze who is affected by this news/text. Respond ONLY in this exact format:
AFFECTED: <comma-separated list from: individuals, business, government, investors, consumers>

Rules:
- Select one or more values that apply
- Use only the exact values listed above
- If none apply, respond with "none"

Example response:
AFFECTED: business, investors, consumers

Text to analyze:`

const EntitiesPrompt = `Extract named entities from the following text. Respond ONLY in this exact format:
PERSONS: <comma-separated list of person names>
ORGANIZATIONS: <comma-separated list of organization names>
LOCATIONS: <comma-separated list of locations/places>
DATES: <comma-separated list of dates/time references>
AMOUNTS: <comma-separated list of monetary amounts, percentages, numbers>

Normalization rules (CRITICAL - follow strictly):

LANGUAGE & GRAMMAR:
- ALL entities MUST be in nominative case / base form (именительный падеж), regardless of how they appear in text
- If text says "в Венесуэле" -> write "Венесуэла", if "Николаса Мадуро" -> write "Николас Мадуро"
- If text says "федеральной резервной системы" -> write "Федеральная резервная система"
- No prepositions, no articles before entities

PERSONS:
- Full name only, one entry per person (e.g. "Дональд Трамп", not "Трамп" as a separate entry)
- Nominative case: "Николас Мадуро" not "Николаса Мадуро"

ORGANIZATIONS:
- Only proper names of companies, agencies, exchanges, media outlets
- Do NOT include: common nouns, country names, product names, game titles, blockchain/crypto network names
- Nominative case: "Нью-Йоркская фондовая биржа" not "Нью-Йоркской фондовой бирже"

LOCATIONS:
- Only geographic places (countries, cities, regions)
- Do NOT include: company names, blockchain names, platform names
- Nominative case: "США" not "в США", "Венесуэла" not "Венесуэлы"
- No duplicates (same place in different cases = one entry)

DATES:
- Bare dates without prepositions: "январь 2024" not "в январе 2024", "2025 год" not "с 2025 года"
- Consistent format, nominative case

AMOUNTS:
- Extract complete numbers with context (e.g. "$1.5 trillion" not "$1" and "5 trillion" separately)
- Include currency symbols. Do NOT extract page numbers, article IDs, or non-meaningful numbers

GENERAL:
- No duplicates in any category
- Use "none" if no entities found for a category

Example response (Russian text):
PERSONS: Дональд Трамп, Николас Мадуро
ORGANIZATIONS: Polymarket, Dow Jones, Федеральная резервная система
LOCATIONS: США, Венесуэла, Нью-Йорк
DATES: январь 2024, 2 квартал 2025
AMOUNTS: $5 млн, 15%, 1000 единиц

Text to analyze:`

const EventsPrompt = `Extract key events mentioned in the following text. Respond ONLY in this exact format:
EVENTS: <semicolon-separated list of events>

Rules:
- Each event should be a brief phrase (3-10 words)
- Extract only actual events/actions mentioned
- Include who/what and action when possible
- Maximum 5 most important events
- Use the same language as the analyzed text
- Use nominative case for subjects (именительный падеж)
- Use "none" if no clear events found

Example response:
EVENTS: company announced quarterly results; CEO resigned from position; new product launched in Europe

Text to analyze:`

const SensationalismPrompt = `Analyze the sensationalism level of the following text. Respond ONLY in this exact format:
SENSATIONALISM: <type> (<confidence 0.0-1.0>)
MARKERS: <comma-separated list of detected markers>

Types (choose one):
- neutral: factual, balanced reporting without emotional language
- emotional: emotionally charged language, dramatic descriptions
- clickbait: exaggerated headlines, curiosity gaps, misleading hooks
- manipulative: deliberate distortion, fear-mongering, propaganda techniques

Markers to detect:
- exaggeration, superlatives, fear_appeal, urgency, misleading_headline, emotional_language, unverified_claims, sensational_words

Example response:
SENSATIONALISM: clickbait (0.8)
MARKERS: exaggeration, misleading_headline, urgency

Text to analyze:`

const UsefulnessPrompt = `Analyze if the following text contains useful, substantive content for readers. Respond ONLY in this exact format:
USEFULNESS: <useful|useless> (<confidence 0.0-1.0>)
REASONS: <comma-separated list of reasons>

Criteria for USELESS content:
- Advertising or promotional material without news value
- Sponsored content disguised as news
- Empty announcements without details
- Repetitive content with no new information
- Clickbait with no substance
- Auto-generated content with no value
- Content that is just a list of links
- Content too short to be informative (less than 2-3 sentences of actual content)
- Press releases with no editorial value
- Content that only announces future events without details

Criteria for USEFUL content:
- Contains factual information or analysis
- Provides new insights or perspectives
- Includes verifiable data or statistics
- Has educational or informational value
- Reports on actual events with details

Example responses:
USEFULNESS: useless (0.9)
REASONS: advertising, promotional_content, no_news_value

USEFULNESS: useful (0.8)
REASONS: factual_information, contains_analysis, new_insights

Text to analyze:`

func BuildCombinedPrompt(req CombinedAnalysisRequest) string {
	var sections []string

	sections = append(sections, "Analyze the following text and provide results for ALL requested sections below.")
	sections = append(sections, "Respond in the EXACT format specified for each section. Each section starts on a new line.")
	sections = append(sections, "")

	if req.Sentiment {
		sections = append(sections, "=== SENTIMENT ===")
		sections = append(sections, "SENTIMENT: <positive|negative|neutral> (<score from -1.0 to 1.0>)")
		sections = append(sections, "Rules: round score to 1 decimal place. Factual reporting = neutral (near 0.0).")
		sections = append(sections, "")
	}

	if req.TagsCount > 0 {
		sections = append(sections, "=== TAGS ===")
		sections = append(sections, fmt.Sprintf("TAGS: <extract %d most important tags, comma-separated>", req.TagsCount))
		sections = append(sections, "Rules: same language as text, lowercase, nominative case, nouns/noun phrases, no duplicates.")
		sections = append(sections, "")
	}

	if req.Classify {
		sections = append(sections, "=== CLASSIFY ===")
		sections = append(sections, "TOPICS: <from: politics, economics, technology, medicine, incidents>")
		sections = append(sections, "SCOPE: <from: regional, international>")
		sections = append(sections, "TYPE: <from: corporate, regulatory, macro>")
		sections = append(sections, "")
	}

	if req.Emotions {
		sections = append(sections, "=== EMOTIONS ===")
		sections = append(sections, "EMOTIONS: <fear:X.X, anger:X.X, hope:X.X, uncertainty:X.X, optimism:X.X, panic:X.X>")
		sections = append(sections, "Rules: only emotions > 0.1, round to 1 decimal, descending order. Be conservative for factual news.")
		sections = append(sections, "")
	}

	if req.Factuality {
		sections = append(sections, "=== FACTUALITY ===")
		sections = append(sections, "FACTUALITY: <confirmed|rumors|forecasts|unsourced> (<confidence 0.0-1.0>)")
		sections = append(sections, "EVIDENCE: <comma-separated: official_source, statistics, quotes, documents, expert_opinion, anonymous_source, speculation, prediction>")
		sections = append(sections, "")
	}

	if req.Impact {
		sections = append(sections, "=== IMPACT ===")
		sections = append(sections, "AFFECTED: <from: individuals, business, government, investors, consumers>")
		sections = append(sections, "")
	}

	if req.Sensationalism {
		sections = append(sections, "=== SENSATIONALISM ===")
		sections = append(sections, "SENSATIONALISM: <neutral|emotional|clickbait|manipulative> (<confidence 0.0-1.0>)")
		sections = append(sections, "MARKERS: <comma-separated markers>")
		sections = append(sections, "")
	}

	if req.Usefulness {
		sections = append(sections, "=== USEFULNESS ===")
		sections = append(sections, "USEFULNESS: <useful|useless> (<confidence 0.0-1.0>)")
		sections = append(sections, "REASONS: <comma-separated reasons>")
		sections = append(sections, "")
	}

	if req.Entities {
		sections = append(sections, "=== ENTITIES ===")
		sections = append(sections, "PERSONS: <comma-separated, full names, nominative case, no duplicates>")
		sections = append(sections, "ORGANIZATIONS: <comma-separated, proper names only, no common nouns/countries/blockchains>")
		sections = append(sections, "LOCATIONS: <comma-separated, geographic places only, nominative case>")
		sections = append(sections, "DATES: <comma-separated, bare dates without prepositions, nominative case>")
		sections = append(sections, "AMOUNTS: <comma-separated, complete numbers with currency>")
		sections = append(sections, "")
	}

	if req.Events {
		sections = append(sections, "=== EVENTS ===")
		sections = append(sections, "EVENTS: <semicolon-separated, 3-10 words each, max 5 events, same language as text>")
		sections = append(sections, "")
	}

	if req.TimeFocus {
		sections = append(sections, "=== TIME FOCUS ===")
		sections = append(sections, "TIME_FOCUS: <past|present|future|mixed> (<confidence 0.0-1.0>)")
		sections = append(sections, "IS_PREDICTION: <true|false>")
		sections = append(sections, "INDICATORS: <comma-separated temporal indicators>")
		sections = append(sections, "Rules: past=already happened, present=current/ongoing, future=predictions/forecasts, mixed=multiple timeframes.")
		sections = append(sections, "IS_PREDICTION=true only for explicit predictions/forecasts/speculations about future outcomes.")
		sections = append(sections, "")
	}

	if req.AdDetect {
		sections = append(sections, "=== AD DETECT ===")
		sections = append(sections, "AD_TYPE: <none|direct|native|sponsored|pr> (<confidence 0.0-1.0>)")
		sections = append(sections, "AD_MARKERS: <comma-separated advertising indicators>")
		sections = append(sections, "Rules: genuine news about companies is NOT advertising. Be strict.")
		sections = append(sections, "")
	}

	sections = append(sections, "Text to analyze:")

	return strings.Join(sections, "\n")
}

func ParseCombinedResponse(response string, req CombinedAnalysisRequest) CombinedAnalysisResponse {
	result := CombinedAnalysisResponse{}

	if req.Sentiment {
		if s, err := ParseSentimentResponse(response); err == nil {
			result.Sentiment = &s
		}
	}

	if req.TagsCount > 0 {
		if t, err := ParseTagsResponse(response); err == nil {
			result.Tags = &t
		}
	}

	if req.Classify {
		if c, err := ParseClassifyResponse(response); err == nil {
			result.Classify = &c
		}
	}

	if req.Emotions {
		if e, err := ParseEmotionsResponse(response); err == nil {
			result.Emotions = &e
		}
	}

	if req.Factuality {
		if f, err := ParseFactualityResponse(response); err == nil {
			result.Factuality = &f
		}
	}

	if req.Impact {
		if i, err := ParseImpactResponse(response); err == nil {
			result.Impact = &i
		}
	}

	if req.Sensationalism {
		if s, err := ParseSensationalismResponse(response); err == nil {
			result.Sensationalism = &s
		}
	}

	if req.Usefulness {
		if u, err := ParseUsefulnessResponse(response); err == nil {
			result.Usefulness = &u
		}
	}

	if req.Entities {
		if e, err := ParseEntitiesResponse(response); err == nil {
			result.Entities = &e
		}
	}

	if req.Events {
		if e, err := ParseEventsResponse(response); err == nil {
			result.Events = &e
		}
	}

	if req.TimeFocus {
		if tf, err := ParseTimeFocusResponse(response); err == nil {
			result.TimeFocus = &tf
		}
	}

	if req.AdDetect {
		if ad, err := ParseAdDetectResponse(response); err == nil {
			result.AdDetect = &ad
		}
	}

	return result
}

type Provider interface {
	Name() string
	Translate(ctx context.Context, req TranslateRequest) (TranslateResponse, error)
	AnalyzeSentiment(ctx context.Context, text string) (SentimentResponse, error)
	ExtractTags(ctx context.Context, text string, count int) (TagsResponse, error)
	Classify(ctx context.Context, text string) (ClassifyResponse, error)
	AnalyzeEmotions(ctx context.Context, text string) (EmotionsResponse, error)
	AnalyzeFactuality(ctx context.Context, text string) (FactualityResponse, error)
	AnalyzeImpact(ctx context.Context, text string) (ImpactResponse, error)
	AnalyzeSensationalism(ctx context.Context, text string) (SensationalismResponse, error)
	AnalyzeUsefulness(ctx context.Context, text string) (UsefulnessResponse, error)
	ExtractEntities(ctx context.Context, text string) (EntitiesResponse, error)
	ExtractEvents(ctx context.Context, text string) (EventsResponse, error)
	AnalyzeTimeFocus(ctx context.Context, text string) (TimeFocusResponse, error)
	AnalyzeAdDetect(ctx context.Context, text string) (AdDetectResponse, error)
	AnalyzeCombined(ctx context.Context, req CombinedAnalysisRequest) (CombinedAnalysisResponse, error)
	ValidateConfig() error
}

type CombinedAnalysisRequest struct {
	Text           string
	Sentiment      bool
	TagsCount      int
	Classify       bool
	Emotions       bool
	Factuality     bool
	Impact         bool
	Sensationalism bool
	Usefulness     bool
	Entities       bool
	Events         bool
	TimeFocus      bool
	AdDetect       bool
}

type CombinedAnalysisResponse struct {
	Sentiment      *SentimentResponse
	Tags           *TagsResponse
	Classify       *ClassifyResponse
	Emotions       *EmotionsResponse
	Factuality     *FactualityResponse
	Impact         *ImpactResponse
	Sensationalism *SensationalismResponse
	Usefulness     *UsefulnessResponse
	Entities       *EntitiesResponse
	Events         *EventsResponse
	TimeFocus      *TimeFocusResponse
	AdDetect       *AdDetectResponse
}

type TranslateRequest struct {
	Text           string
	SourceLang     string
	TargetLang     string
	Style          string
	Context        string
	Glossary       []config.GlossaryEntry
	Temperature    float64
	MaxTokens      int
	PreserveFormat bool
}

type TranslateResponse struct {
	Text         string
	DetectedLang string
	TokensUsed   int
}

type SentimentResponse struct {
	Sentiment  string  // positive, negative, neutral
	Score      float64 // -1.0 to 1.0
	Confidence float64 // 0.0 to 1.0
}

type TagsResponse struct {
	Tags []string
}

type ClassifyResponse struct {
	Topics   []string // politics, economics, technology, medicine, incidents
	Scope    []string // regional, international
	NewsType []string // corporate, regulatory, macro
}

type EmotionsResponse struct {
	Emotions map[string]float64 // emotion -> intensity (0.0-1.0)
}

type FactualityResponse struct {
	Type       string   // confirmed, rumors, forecasts, unsourced
	Confidence float64  // 0.0-1.0
	Evidence   []string // evidence types found
}

type ImpactResponse struct {
	Affected []string // individuals, business, government, investors, consumers
}

type SensationalismResponse struct {
	Type       string   // neutral, emotional, clickbait, manipulative
	Confidence float64  // 0.0-1.0
	Markers    []string // detected sensationalism markers
}

type EntitiesResponse struct {
	Persons       []string // person names
	Organizations []string // organization names
	Locations     []string // locations/places
	Dates         []string // dates/time references
	Amounts       []string // monetary amounts, percentages, numbers
}

type EventsResponse struct {
	Events []string // key events mentioned
}

type UsefulnessResponse struct {
	IsUseful   bool     // true if useful, false if useless
	Confidence float64  // 0.0-1.0
	Reasons    []string // reasons for the decision
}

type TimeFocusResponse struct {
	Focus        string   // past, present, future, mixed
	Confidence   float64  // 0.0-1.0
	IsPrediction bool     // true if text contains predictions/forecasts
	Indicators   []string // temporal indicators found in text
}

type AdDetectResponse struct {
	AdType     string   // none, direct, native, sponsored, pr
	Confidence float64  // 0.0-1.0
	Markers    []string // advertising indicators found in text
}

type BaseProvider struct {
	name       string
	config     config.ProviderConfig
	httpClient *http.Client
}

func (b *BaseProvider) Name() string {
	return b.name
}

func (b *BaseProvider) ValidateConfig() error {
	if b.config.BaseURL == "" {
		return fmt.Errorf("base URL is required for provider %s", b.name)
	}
	if b.name != "ollama" && b.config.APIKey == "" {
		return fmt.Errorf("API key is required for provider %s", b.name)
	}
	return nil
}

func (b *BaseProvider) buildPrompt(req TranslateRequest, systemPrompt string) string {
	prompt := systemPrompt

	if req.Context != "" {
		prompt += "\n\nContext: " + req.Context
	}

	if req.Style != "" {
		stylePrompts := map[string]string{
			"formal":    "Use formal language appropriate for official documents.",
			"informal":  "Use casual, conversational language.",
			"technical": "Preserve technical terminology accurately.",
			"literary":  "Maintain literary style and artistic expression.",
		}
		if stylePrompt, ok := stylePrompts[req.Style]; ok {
			prompt += "\n\n" + stylePrompt
		}
	}

	if len(req.Glossary) > 0 {
		prompt += "\n\nGlossary (use these translations):\n"
		for _, entry := range req.Glossary {
			source := entry.Source
			target := entry.Target
			if source == "" {
				source = entry.Term
			}
			if target == "" {
				target = entry.Translation
			}
			if source != "" && target != "" {
				prompt += fmt.Sprintf("- %s -> %s", source, target)
				if entry.Note != "" {
					prompt += " (" + entry.Note + ")"
				}
				prompt += "\n"
			}
		}
	}

	if req.PreserveFormat {
		prompt += "\n\nPreserve all formatting including markdown, HTML tags, and code blocks."
	}

	return prompt
}

type Registry struct {
	providers map[string]func(config.ProviderConfig, *http.Client) Provider
}

var registry = &Registry{
	providers: make(map[string]func(config.ProviderConfig, *http.Client) Provider),
}

func Register(name string, factory func(config.ProviderConfig, *http.Client) Provider) {
	registry.providers[name] = factory
}

func Get(name string, cfg config.ProviderConfig, client *http.Client) (Provider, error) {
	factory, ok := registry.providers[name]
	if !ok {
		return nil, fmt.Errorf("unknown provider: %s", name)
	}

	provider := factory(cfg, client)
	if err := provider.ValidateConfig(); err != nil {
		return nil, err
	}

	return provider, nil
}

func ListProviders() []string {
	names := make([]string, 0, len(registry.providers))
	for name := range registry.providers {
		names = append(names, name)
	}
	return names
}

func ParseSentimentResponse(response string) (SentimentResponse, error) {
	response = strings.TrimSpace(response)

	re := regexp.MustCompile(`(?im)^SENTIMENT:\s*(positive|negative|neutral)\s*\(([+-]?\d*\.?\d+)\)`)
	matches := re.FindStringSubmatch(response)

	if len(matches) < 3 {
		return SentimentResponse{}, fmt.Errorf("invalid sentiment response format: %s", response)
	}

	sentiment := strings.ToLower(matches[1])
	score, err := strconv.ParseFloat(matches[2], 64)
	if err != nil {
		return SentimentResponse{}, fmt.Errorf("invalid score: %w", err)
	}

	confidence := 1.0 - (1.0-abs(score))*0.5

	return SentimentResponse{
		Sentiment:  sentiment,
		Score:      score,
		Confidence: confidence,
	}, nil
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}

func ParseTagsResponse(response string) (TagsResponse, error) {
	response = strings.TrimSpace(response)

	re := regexp.MustCompile(`(?im)^TAGS:\s*(.+)`)
	matches := re.FindStringSubmatch(response)

	if len(matches) < 2 {
		return TagsResponse{}, fmt.Errorf("invalid tags response format: %s", response)
	}

	tagsStr := matches[1]
	rawTags := strings.Split(tagsStr, ",")

	var tags []string
	for _, tag := range rawTags {
		tag = strings.TrimSpace(tag)
		if tag != "" {
			tags = append(tags, tag)
		}
	}

	if len(tags) == 0 {
		return TagsResponse{}, fmt.Errorf("no tags found in response")
	}

	return TagsResponse{Tags: tags}, nil
}

func ParseClassifyResponse(response string) (ClassifyResponse, error) {
	response = strings.TrimSpace(response)
	result := ClassifyResponse{}

	// Parse TOPICS
	topicsRe := regexp.MustCompile(`(?im)^TOPICS:\s*(.+)`)
	if matches := topicsRe.FindStringSubmatch(response); len(matches) >= 2 {
		result.Topics = parseCommaSeparated(matches[1])
	}

	// Parse SCOPE
	scopeRe := regexp.MustCompile(`(?im)^SCOPE:\s*(.+)`)
	if matches := scopeRe.FindStringSubmatch(response); len(matches) >= 2 {
		result.Scope = parseCommaSeparated(matches[1])
	}

	// Parse TYPE
	typeRe := regexp.MustCompile(`(?im)^TYPE:\s*(.+)`)
	if matches := typeRe.FindStringSubmatch(response); len(matches) >= 2 {
		result.NewsType = parseCommaSeparated(matches[1])
	}

	if len(result.Topics) == 0 && len(result.Scope) == 0 && len(result.NewsType) == 0 {
		return ClassifyResponse{}, fmt.Errorf("invalid classify response format: %s", response)
	}

	return result, nil
}

func parseCommaSeparated(s string) []string {
	var result []string
	parts := strings.Split(s, ",")
	for _, p := range parts {
		p = strings.TrimSpace(strings.ToLower(p))
		if p != "" && p != "none" {
			result = append(result, p)
		}
	}
	return result
}

// parseCommaSeparatedKeepCase splits comma-separated values preserving original case.
// Used for entities where capitalization matters (person names, organizations, etc).
func parseCommaSeparatedKeepCase(s string) []string {
	var result []string
	parts := strings.Split(s, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		lower := strings.ToLower(p)
		if p != "" && lower != "none" {
			result = append(result, p)
		}
	}
	return result
}

// filterAmounts removes garbage from extracted amounts list.
// Filters out: standalone bare numbers (no currency/unit), very short fragments.
var amountHasContextRe = regexp.MustCompile(`(?i)[\$€£¥₽%]|млн|млрд|тыс|трлн|million|billion|thousand|trillion|процент|percent|доллар|dollar|евро|euro|рубл|рубле|фунт|pound|иен|yen|юан|yuan`)

func filterAmounts(amounts []string) []string {
	var result []string
	for _, a := range amounts {
		a = strings.TrimSpace(a)
		if a == "" {
			continue
		}
		// Skip standalone bare numbers without any currency/unit context
		if isBareNumber(a) {
			continue
		}
		// Skip very short fragments (1-2 chars) like "$" or "4"
		if len([]rune(a)) <= 2 && !amountHasContextRe.MatchString(a) {
			continue
		}
		result = append(result, a)
	}
	return result
}

// isBareNumber returns true if the string is just a plain number (integer or decimal)
// without any currency symbols, units, or meaningful context.
func isBareNumber(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	// Remove thousands separators (spaces, commas in numbers)
	cleaned := strings.ReplaceAll(s, " ", "")
	cleaned = strings.ReplaceAll(cleaned, ",", "")
	_, err := strconv.ParseFloat(cleaned, 64)
	return err == nil
}

func ParseEmotionsResponse(response string) (EmotionsResponse, error) {
	response = strings.TrimSpace(response)
	result := EmotionsResponse{
		Emotions: make(map[string]float64),
	}

	re := regexp.MustCompile(`(?im)^EMOTIONS:\s*(.+)`)
	matches := re.FindStringSubmatch(response)

	if len(matches) < 2 {
		return EmotionsResponse{}, fmt.Errorf("invalid emotions response format: %s", response)
	}

	emotionRe := regexp.MustCompile(`(\w+):([0-9.]+)`)
	emotionMatches := emotionRe.FindAllStringSubmatch(matches[1], -1)

	for _, m := range emotionMatches {
		if len(m) >= 3 {
			emotion := strings.ToLower(m[1])
			score, err := strconv.ParseFloat(m[2], 64)
			if err == nil && score > 0 {
				result.Emotions[emotion] = score
			}
		}
	}

	if len(result.Emotions) == 0 {
		return EmotionsResponse{}, fmt.Errorf("no emotions found in response")
	}

	return result, nil
}

func ParseFactualityResponse(response string) (FactualityResponse, error) {
	response = strings.TrimSpace(response)
	result := FactualityResponse{}

	// Parse FACTUALITY line
	factRe := regexp.MustCompile(`(?im)^FACTUALITY:\s*(\w+)\s*\(([0-9.]+)\)`)
	if matches := factRe.FindStringSubmatch(response); len(matches) >= 3 {
		result.Type = strings.ToLower(matches[1])
		if score, err := strconv.ParseFloat(matches[2], 64); err == nil {
			result.Confidence = score
		}
	}

	// Parse EVIDENCE line
	evidenceRe := regexp.MustCompile(`(?im)^EVIDENCE:\s*(.+)`)
	if matches := evidenceRe.FindStringSubmatch(response); len(matches) >= 2 {
		result.Evidence = parseCommaSeparated(matches[1])
	}

	if result.Type == "" {
		return FactualityResponse{}, fmt.Errorf("invalid factuality response format: %s", response)
	}

	return result, nil
}

func ParseImpactResponse(response string) (ImpactResponse, error) {
	response = strings.TrimSpace(response)
	result := ImpactResponse{}

	re := regexp.MustCompile(`(?im)^AFFECTED:\s*(.+)`)
	matches := re.FindStringSubmatch(response)

	if len(matches) < 2 {
		return ImpactResponse{}, fmt.Errorf("invalid impact response format: %s", response)
	}

	result.Affected = parseCommaSeparated(matches[1])

	return result, nil
}

func ParseSensationalismResponse(response string) (SensationalismResponse, error) {
	response = strings.TrimSpace(response)
	result := SensationalismResponse{}

	// Parse SENSATIONALISM line
	sensRe := regexp.MustCompile(`(?im)^SENSATIONALISM:\s*(\w+)\s*\(([0-9.]+)\)`)
	if matches := sensRe.FindStringSubmatch(response); len(matches) >= 3 {
		result.Type = strings.ToLower(matches[1])
		if score, err := strconv.ParseFloat(matches[2], 64); err == nil {
			result.Confidence = score
		}
	}

	// Parse MARKERS line
	markersRe := regexp.MustCompile(`(?im)^MARKERS:\s*(.+)`)
	if matches := markersRe.FindStringSubmatch(response); len(matches) >= 2 {
		result.Markers = parseCommaSeparated(matches[1])
	}

	if result.Type == "" {
		return SensationalismResponse{}, fmt.Errorf("invalid sensationalism response format: %s", response)
	}

	return result, nil
}

func ParseEntitiesResponse(response string) (EntitiesResponse, error) {
	response = strings.TrimSpace(response)
	result := EntitiesResponse{}

	// Parse PERSONS (keep case for proper names)
	personsRe := regexp.MustCompile(`(?im)^PERSONS:\s*(.+)`)
	if matches := personsRe.FindStringSubmatch(response); len(matches) >= 2 {
		result.Persons = parseCommaSeparatedKeepCase(matches[1])
	}

	// Parse ORGANIZATIONS (keep case for proper names)
	orgsRe := regexp.MustCompile(`(?im)^ORGANIZATIONS:\s*(.+)`)
	if matches := orgsRe.FindStringSubmatch(response); len(matches) >= 2 {
		result.Organizations = parseCommaSeparatedKeepCase(matches[1])
	}

	// Parse LOCATIONS (keep case for proper names)
	locsRe := regexp.MustCompile(`(?im)^LOCATIONS:\s*(.+)`)
	if matches := locsRe.FindStringSubmatch(response); len(matches) >= 2 {
		result.Locations = parseCommaSeparatedKeepCase(matches[1])
	}

	// Parse DATES (keep case for date formatting)
	datesRe := regexp.MustCompile(`(?im)^DATES:\s*(.+)`)
	if matches := datesRe.FindStringSubmatch(response); len(matches) >= 2 {
		result.Dates = parseCommaSeparatedKeepCase(matches[1])
	}

	// Parse AMOUNTS (keep case, then filter garbage)
	amountsRe := regexp.MustCompile(`(?im)^AMOUNTS:\s*(.+)`)
	if matches := amountsRe.FindStringSubmatch(response); len(matches) >= 2 {
		result.Amounts = filterAmounts(parseCommaSeparatedKeepCase(matches[1]))
	}

	if len(result.Persons) == 0 && len(result.Organizations) == 0 && len(result.Locations) == 0 && len(result.Dates) == 0 && len(result.Amounts) == 0 {
		return EntitiesResponse{}, fmt.Errorf("no entities found in response: %s", response)
	}

	return result, nil
}

func ParseEventsResponse(response string) (EventsResponse, error) {
	response = strings.TrimSpace(response)
	result := EventsResponse{}

	re := regexp.MustCompile(`(?im)^EVENTS:\s*(.+)`)
	matches := re.FindStringSubmatch(response)

	if len(matches) >= 2 {
		eventsStr := matches[1]
		if strings.ToLower(strings.TrimSpace(eventsStr)) != "none" {
			parts := strings.Split(eventsStr, ";")
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p != "" && strings.ToLower(p) != "none" {
					result.Events = append(result.Events, p)
				}
			}
		}
	}

	if len(result.Events) == 0 {
		return EventsResponse{}, fmt.Errorf("no events found in response: %s", response)
	}

	return result, nil
}

func ParseUsefulnessResponse(response string) (UsefulnessResponse, error) {
	response = strings.TrimSpace(response)
	result := UsefulnessResponse{}

	// Parse USEFULNESS line
	usefulRe := regexp.MustCompile(`(?im)^USEFULNESS:\s*(useful|useless)\s*\(([0-9.]+)\)`)
	if matches := usefulRe.FindStringSubmatch(response); len(matches) >= 3 {
		result.IsUseful = strings.ToLower(matches[1]) == "useful"
		if score, err := strconv.ParseFloat(matches[2], 64); err == nil {
			result.Confidence = score
		}
	} else {
		return UsefulnessResponse{}, fmt.Errorf("invalid usefulness response format: %s", response)
	}

	// Parse REASONS line
	reasonsRe := regexp.MustCompile(`(?im)^REASONS:\s*(.+)`)
	if matches := reasonsRe.FindStringSubmatch(response); len(matches) >= 2 {
		result.Reasons = parseCommaSeparated(matches[1])
	}

	return result, nil
}

func ParseTimeFocusResponse(response string) (TimeFocusResponse, error) {
	response = strings.TrimSpace(response)
	result := TimeFocusResponse{}

	// Parse TIME_FOCUS line
	focusRe := regexp.MustCompile(`(?im)^TIME_FOCUS:\s*(past|present|future|mixed)\s*\(([0-9.]+)\)`)
	if matches := focusRe.FindStringSubmatch(response); len(matches) >= 3 {
		result.Focus = strings.ToLower(matches[1])
		if score, err := strconv.ParseFloat(matches[2], 64); err == nil {
			result.Confidence = score
		}
	} else {
		return TimeFocusResponse{}, fmt.Errorf("invalid time focus response format: %s", response)
	}

	// Parse IS_PREDICTION line
	predRe := regexp.MustCompile(`(?im)^IS_PREDICTION:\s*(true|false)`)
	if matches := predRe.FindStringSubmatch(response); len(matches) >= 2 {
		result.IsPrediction = strings.ToLower(matches[1]) == "true"
	}

	// Parse INDICATORS line
	indRe := regexp.MustCompile(`(?im)^INDICATORS:\s*(.+)`)
	if matches := indRe.FindStringSubmatch(response); len(matches) >= 2 {
		result.Indicators = parseCommaSeparated(matches[1])
	}

	return result, nil
}

func ParseAdDetectResponse(response string) (AdDetectResponse, error) {
	response = strings.TrimSpace(response)
	result := AdDetectResponse{}

	// Parse AD_TYPE line
	adRe := regexp.MustCompile(`(?im)^AD_TYPE:\s*(none|direct|native|sponsored|pr)\s*\(([0-9.]+)\)`)
	if matches := adRe.FindStringSubmatch(response); len(matches) >= 3 {
		result.AdType = strings.ToLower(matches[1])
		if score, err := strconv.ParseFloat(matches[2], 64); err == nil {
			result.Confidence = score
		}
	} else {
		return AdDetectResponse{}, fmt.Errorf("invalid ad detect response format: %s", response)
	}

	// Parse AD_MARKERS line
	markersRe := regexp.MustCompile(`(?im)^AD_MARKERS:\s*(.+)`)
	if matches := markersRe.FindStringSubmatch(response); len(matches) >= 2 {
		markers := parseCommaSeparated(matches[1])
		if len(markers) == 1 && markers[0] == "none" {
			markers = nil
		}
		result.Markers = markers
	}

	return result, nil
}
