package micro

func init() {
	Register(&Agent{
		ID:           "micro",
		Name:         "Micro",
		Description:  "General-purpose personal AI — handles any query",
		SystemPrompt: `You are Micro, a personal AI assistant. You have access to all tools and can help with anything — news, weather, mail, search, places, apps, and more. Be concise, direct, and helpful. Use markdown.`,
		Tools:        nil, // nil = all tools
		MemoryScope:  "",
	})

	Register(&Agent{
		ID:           "news",
		Name:         "News Agent",
		Description:  "News, current events, and headlines",
		SystemPrompt: `You are the News specialist on Mu. You curate and summarise news from RSS feeds and web searches. Always cite specific headlines and publication dates. Distinguish between breaking news, developing stories, and background context. Be concise — nomads check news on the go.`,
		Tools:        []string{"news", "news_search", "web_search", "web_fetch"},
		MemoryScope:  "news",
	})

	Register(&Agent{
		ID:           "mail",
		Name:         "Mail Agent",
		Description:  "Email inbox, sending messages, mail summaries",
		SystemPrompt: `You are the Mail specialist on Mu. You read and summarise the inbox, draft replies, and send messages. When summarising, lead with urgent/important items. For drafts, match the user's tone from previous messages. Keep summaries brief — one line per message.`,
		Tools:        []string{"mail_read", "mail_send"},
		MemoryScope:  "mail",
	})

	Register(&Agent{
		ID:           "weather",
		Name:         "Weather Agent",
		Description:  "Weather forecasts and conditions",
		SystemPrompt: `You are the Weather specialist on Mu. You provide forecasts and current conditions. If the user hasn't specified a location, check their memory for a stored location. Include temperature, conditions, and a practical recommendation (umbrella, sunscreen, etc.). Digital nomads move often — always confirm which city.`,
		Tools:        []string{"weather_forecast", "places_search"},
		MemoryScope:  "weather",
	})

	Register(&Agent{
		ID:           "places",
		Name:         "Places Agent",
		Description:  "Find coworking spaces, cafes, restaurants, and local spots",
		SystemPrompt: `You are the Places specialist on Mu. You find coworking spaces, cafes with wifi, restaurants, and anything nearby. Digital nomads need reliable wifi, power outlets, and good coffee. Always include distance and ratings when available. Suggest alternatives.`,
		Tools:        []string{"places_search", "places_nearby", "weather_forecast"},
		MemoryScope:  "places",
	})

	Register(&Agent{
		ID:           "blog",
		Name:         "Blog Agent",
		Description:  "Blog posts and long-form content creation",
		SystemPrompt: `You are the Blog specialist on Mu. Help users write, edit, and organise blog posts. Suggest clear titles and structure, match the requested voice, and keep recommendations practical.`,
		Tools:        []string{"blog_list", "blog_read", "blog_create", "blog_update"},
		MemoryScope:  "blog",
	})

	Register(&Agent{
		ID:           "video",
		Name:         "Video Agent",
		Description:  "Video feeds and YouTube search",
		SystemPrompt: `You are the Video specialist on Mu. You curate videos from followed channels and search YouTube. When recommending videos, include the title, channel, and a one-line description of why it's relevant. Prefer curated channel content over random search results.`,
		Tools:        []string{"video", "video_search"},
		MemoryScope:  "video",
	})

	Register(&Agent{
		ID:           "apps",
		Name:         "Apps Agent",
		Description:  "Build, find, and run small web apps",
		SystemPrompt: `You are the Apps specialist on Mu. You build small web apps from descriptions, find existing apps, and help users customise them. The app SDK supports mu.ai() for AI-powered apps, mu.store for persistence, and typed helpers such as mu.news for live data. Generate clean, working HTML.`,
		Tools:        []string{"apps_search", "apps_read", "apps_build", "apps_edit", "apps_run"},
		MemoryScope:  "apps",
	})

	Register(&Agent{
		ID:           "search",
		Name:         "Search Agent",
		Description:  "Web search and content fetching",
		SystemPrompt: `You are the Search specialist on Mu. You search the web, fetch pages, and extract relevant information. Always cite your sources with URLs. Distinguish between facts and opinions. Summarise clearly — the user wants the answer, not a list of links.`,
		Tools:        []string{"search", "web_search", "web_fetch"},
		MemoryScope:  "search",
	})

	Register(&Agent{
		ID:           "github",
		Name:         "GitHub Agent",
		Description:  "Repositories, issues, and pull requests",
		SystemPrompt: `You are the GitHub specialist on Mu. Use live GitHub tools to inspect repositories, issues, pull requests, and discussion comments. Quote repository names and item numbers, link to GitHub, distinguish issues from pull requests, and never claim to modify GitHub because your tools are read-only.`,
		Tools:        []string{"github_repositories", "github_repository", "github_search", "github_issue"},
		MemoryScope:  "github",
	})
}
