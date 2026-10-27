package service

import (
	"bytes"
	_ "embed"
	"html/template"
	"strings"
)

type telemtWebArticle struct{ Number, Title, Text string }
type telemtWebTheme struct {
	Media                                                                              bool
	Layout, Title, Headline, Description, Section, Action, AboutTitle, About, Category string
	Articles                                                                           []telemtWebArticle
}

// These original, self-contained themes use no remote scripts, images or fonts.
var telemtWebThemes = map[string]telemtWebTheme{
	"coffee": {Layout: "warm", Title: "Amber Coffee", Headline: "Good days start slowly.", Description: "Coffee, brewing notes and small rituals for a gentler morning.", Section: "Brewing notes", Action: "Explore the brews", AboutTitle: "A place for a slower cup.", About: "We collect approachable ways to make better coffee at home, one cup at a time.", Category: "Coffee house", Articles: []telemtWebArticle{
		{"01", "The everyday filter", "Start with fresh water and a medium grind. Change one variable at a time to find the cup you enjoy."},
		{"02", "A patient cold brew", "Steep coarse grounds in cool water overnight, strain carefully and serve over ice."},
		{"03", "Keep beans fresh", "Choose a small airtight container and a cool cupboard away from direct sunlight."},
	}},
	"bakery": {Layout: "warm", Title: "Sunday Crumb", Headline: "Made slowly. Shared warmly.", Description: "A baking notebook filled with simple loaves, seasonal treats and flour-dusted afternoons.", Section: "From the oven", Action: "Read the notebook", AboutTitle: "Room at the kitchen table.", About: "These notes celebrate the pleasure of baking and sharing something made by hand.", Category: "Bakery", Articles: []telemtWebArticle{
		{"01", "The first loaf", "Flour, water, yeast and salt are enough to begin. Let the dough rest and learn how it feels."},
		{"02", "Seasonal fruit galette", "Fold a simple pastry around sliced fruit. A rough edge is part of its charm."},
		{"03", "Save a slice", "Freeze cooled bread in slices and toast only what you need for breakfast."},
	}},
	"photography": {Layout: "gallery", Title: "Still Frame", Headline: "See the ordinary differently.", Description: "Studies in light, quiet streets and the moments between destinations.", Section: "Selected studies", Action: "View the studies", AboutTitle: "Looking with intention.", About: "A personal collection of photographic ideas, composed around light, texture and everyday observation.", Category: "Photography", Articles: []telemtWebArticle{
		{"01", "Morning light", "Watch how a familiar room changes when the sun first reaches the window."},
		{"02", "City geometry", "Look for repeating windows, long shadows and the rhythm of a quiet street."},
		{"03", "Small details", "Move closer to the texture of paper, the edge of a leaf or rain on glass."},
	}},
	"architecture": {Layout: "gallery", Title: "Form & Space", Headline: "Space to live well.", Description: "A visual notebook about thoughtful buildings, honest materials and places that feel welcoming.", Section: "Design observations", Action: "Explore the notes", AboutTitle: "Built around everyday life.", About: "We study the small architectural decisions that make rooms and public spaces more comfortable.", Category: "Architecture", Articles: []telemtWebArticle{
		{"01", "Natural light", "A well-placed opening can shape the rhythm of a room throughout the day."},
		{"02", "Honest materials", "Timber, brick and stone carry texture that becomes richer with everyday use."},
		{"03", "Shared spaces", "A bench, a sheltered entrance and a clear walking route can make a street feel inviting."},
	}},
	"books": {Layout: "editorial", Title: "Margin Notes", Headline: "Another page. Another perspective.", Description: "Reading lists, reflections and small discoveries from a well-used bookshelf.", Section: "On the reading desk", Action: "Open the notebook", AboutTitle: "A quiet corner for readers.", About: "This reading room is for thoughtful browsing, revisiting old favourites and making space for new ideas.", Category: "Reading room", Articles: []telemtWebArticle{
		{"01", "Reading without a target", "Choose a book out of curiosity and let yourself linger over a paragraph."},
		{"02", "Notes in the margin", "Keep a separate notebook for questions, connections and sentences to revisit."},
		{"03", "A varied bookshelf", "Alternate fiction, essays and poetry to discover different ways of seeing."},
	}},
	"ceramics": {Layout: "warm", Title: "Earth & Kiln", Headline: "Objects with a human touch.", Description: "Notes from the pottery table: clay, patient practice and useful objects for everyday life.", Section: "Studio notebook", Action: "Visit the notebook", AboutTitle: "Learning through making.", About: "A small studio journal about texture, form and the pleasure of working with simple materials.", Category: "Ceramics studio", Articles: []telemtWebArticle{
		{"01", "Begin with a pinch pot", "Turn a small ball of clay in your palm and gradually shape the walls with gentle pressure."},
		{"02", "Finding a useful shape", "Think about how a cup sits in the hand before adding a handle or decoration."},
		{"03", "A record of surfaces", "Label test tiles and keep notes on clay bodies, glazes and firing results."},
	}},
	"garden": {Layout: "editorial", Title: "Green Corner", Headline: "Grow a little every day.", Description: "A practical journal for windowsills, small gardens and the changing seasons.", Section: "Growing notes", Action: "Find a growing idea", AboutTitle: "Small spaces, growing possibilities.", About: "You do not need a large garden to enjoy watching something grow. A pot and a little attention are a good start.", Category: "Garden journal", Articles: []telemtWebArticle{
		{"01", "Herbs by the window", "Choose a bright spot and a pot with drainage. Check the soil before watering."},
		{"02", "Make room for pollinators", "A few flowering plants can bring colour and welcome visiting insects."},
		{"03", "Keep a seasonal journal", "Record planting dates and small changes to learn what works in your space."},
	}},
	"astronomy": {Layout: "night", Title: "After Dusk", Headline: "Look up. Stay curious.", Description: "An evening notebook about the Moon, constellations and learning to find your way around the sky.", Section: "Under the stars", Action: "Explore the sky", AboutTitle: "Wonder begins overhead.", About: "These notes are for curious observers who enjoy an unhurried evening outside beneath the stars.", Category: "Night sky", Articles: []telemtWebArticle{
		{"01", "Follow the Moon", "Sketch its shape over several evenings and notice how its position changes."},
		{"02", "Start with your eyes", "Find an open view, let your eyes adjust and identify a few familiar patterns."},
		{"03", "Keep an observing log", "Write down the time, cloud cover and what you noticed to compare future nights."},
	}},
	"cooking": {Layout: "editorial", Title: "Season & Spoon", Headline: "Simple food. Full of character.", Description: "Everyday cooking ideas built around seasonal ingredients and a well-stocked cupboard.", Section: "Kitchen ideas", Action: "Find some inspiration", AboutTitle: "Cooking with what is at hand.", About: "A collection of flexible kitchen ideas that leave room for taste, season and the ingredients you already have.", Category: "Kitchen notebook", Articles: []telemtWebArticle{
		{"01", "A tray of vegetables", "Cut vegetables into similar sizes, add oil and seasonings, then roast until tender."},
		{"02", "A brighter bowl", "Build a grain bowl with beans, crunchy vegetables and a lemon dressing."},
		{"03", "Use the whole bunch", "Turn leftover herbs into a quick sauce with oil, citrus and toasted seeds."},
	}},
	"cycling": {Layout: "gallery", Title: "Open Pedal", Headline: "Take the long way home.", Description: "Easy rides, quiet paths and practical notes for exploring the neighbourhood on two wheels.", Section: "Ride notes", Action: "Plan a gentle ride", AboutTitle: "More discovery in every ride.", About: "A notebook for relaxed cycling, small detours and noticing the places close to home.", Category: "Cycling journal", Articles: []telemtWebArticle{
		{"01", "The riverside loop", "Look for a connected path with a comfortable surface and space for a quiet break."},
		{"02", "Before you roll", "Check tyre pressure, brakes and lights. Bring water and a basic repair kit."},
		{"03", "A new perspective", "Try a familiar route in the opposite direction and stop somewhere you usually pass."},
	}},
	"music": {Layout: "night", Title: "Quiet Frequency", Headline: "Make room for listening.", Description: "Album rituals, sound discoveries and small notes on listening with attention.", Section: "Listening notes", Action: "Enter the listening room", AboutTitle: "One record at a time.", About: "A personal space for exploring sound, returning to favourite recordings and listening beyond the background.", Category: "Listening room", Articles: []telemtWebArticle{
		{"01", "Listen to a whole album", "Set aside a little time and follow the sequence from the opening track to the last."},
		{"02", "Notice the layers", "Return to a familiar song and focus on a different instrument or harmony."},
		{"03", "Keep a listening journal", "Write down a mood or detail that stands out, then revisit it on another day."},
	}},
	"coworking": {Layout: "gallery", Title: "Common Desk", Headline: "A little space to do good work.", Description: "Ideas for calm workdays, shared spaces and a more thoughtful daily routine.", Section: "Workspace notes", Action: "Explore the workspace", AboutTitle: "Better days through thoughtful spaces.", About: "Common Desk is a notebook about the routines and environments that help people focus and work together.", Category: "Coworking space", Articles: []telemtWebArticle{
		{"01", "Find your focus", "Begin with one clear task and give it a quiet block of uninterrupted time."},
		{"02", "A considerate shared desk", "Keep calls in a suitable space and leave the desk ready for the next person."},
		{"03", "Make time to reset", "A short walk and a screen break can mark a useful pause between tasks."},
	}},
	"video": {Media: true, Layout: "video", Title: "Frame Library", Headline: "Stories worth your time.", Description: "A curated video journal exploring places, creative work and the people behind everyday ideas.", Section: "Latest stories", Action: "Browse the stories", AboutTitle: "Every story has a beginning.", About: "Frame Library collects independent story ideas and production notes for curious viewers and creators.", Category: "Video magazine", Articles: []telemtWebArticle{
		{"01", "Above the coastline", "A location notebook for a slow journey along cliffs, harbours and the edge of the sea. Start with wide views, then look for details that give a place its character."},
		{"02", "The city before sunrise", "An exploration of the hour before a city wakes: empty platforms, early deliveries and the first light on familiar buildings."},
		{"03", "Inside the workshop", "Follow an object from the first sketch to the finished piece. Hands, tools and small decisions tell the story of making."},
		{"04", "A season in the mountains", "Changing weather reshapes a familiar landscape. A field journal about preparing a route and observing the light."},
		{"05", "Sound of the neighbourhood", "Build a portrait of a place through sound: bicycles, footsteps, open windows and a distant train."},
		{"06", "Behind the frame", "How a sequence comes together: establishing a scene, finding a detail and leaving space for a quiet ending."},
	}},

	"cinema": {Media: true, Layout: "cinema", Title: "Screen Notes", Headline: "A wider world of cinema.", Description: "Film essays, viewing themes and a thoughtful guide to looking beyond the opening scene.", Section: "From the journal", Action: "Explore the journal", AboutTitle: "For the love of the moving image.", About: "Screen Notes is an independent editorial notebook about film language, atmosphere and the experience of watching together.", Category: "Cinema journal", Articles: []telemtWebArticle{
		{"01", "A landscape as a character", "Open spaces can shape a story as strongly as a leading role. Notice how a horizon changes the scale of a scene."},
		{"02", "The art of the opening shot", "An opening image introduces a rhythm and a point of view before the first line is spoken."},
		{"03", "Light after dark", "Street lamps, reflections and deep shadows create different kinds of tension in a night scene."},
		{"04", "Rooms that tell stories", "Set design gives clues about the people who occupy a space, from worn furniture to objects left on a table."},
		{"05", "Listening to silence", "A pause in the soundtrack can draw attention to a gesture or make an ordinary sound feel significant."},
		{"06", "Watching with a notebook", "Record one image and one question after a film. Returning to those notes can reveal patterns in your taste."},
	}},

	"games": {Media: true, Layout: "games", Title: "Level Journal", Headline: "Discover your next world.", Description: "A magazine about game worlds, creative design and the small details that make play memorable.", Section: "On the radar", Action: "Explore the features", AboutTitle: "Curiosity is part of the game.", About: "Level Journal explores the craft of interactive worlds through accessible essays and design observations.", Category: "Games magazine", Articles: []telemtWebArticle{
		{"01", "Worlds beyond the map", "How paths, distant landmarks and hidden corners encourage players to explore without a long checklist."},
		{"02", "A good first level", "Early spaces teach movement and timing through safe experiments, clear feedback and room to try again."},
		{"03", "The quiet side quest", "Small optional stories can make a world feel lived in, especially when they reward attention rather than speed."},
		{"04", "Building a memorable puzzle", "A satisfying puzzle introduces a rule, changes the context and gives the player space to make the connection."},
		{"05", "Sound as a compass", "Footsteps, ambient sound and musical cues can help players understand a space without looking at a marker."},
		{"06", "Play together", "Cooperative design works best when every player has a useful role and a chance to contribute."},
	}},

	"courses": {Media: true, Layout: "courses", Title: "Open Lessons", Headline: "Learn something worth making.", Description: "A self-guided learning library with practical study paths for creative and curious minds.", Section: "Learning paths", Action: "Choose a learning path", AboutTitle: "Small steps, lasting understanding.", About: "Open Lessons organises free reading paths and practical exercises. Start anywhere and work at your own pace.", Category: "Learning library", Articles: []telemtWebArticle{
		{"01", "Photography fundamentals", "A study path through composition, available light and deliberate observation. Exercise: photograph the same subject from five viewpoints."},
		{"02", "Design for everyday life", "Explore hierarchy, spacing and useful constraints. Exercise: simplify a notice so its most important message can be read at a glance."},
		{"03", "A first creative journal", "Build a routine of collecting questions, sketches and observations. Exercise: make one small entry every day for a week."},
		{"04", "Understand the night sky", "Learn directions, Moon phases and a few seasonal star patterns. Exercise: keep an evening observing log from the same location."},
		{"05", "Tell a short visual story", "Plan a beginning, a change and an ending using a small set of images. Exercise: tell an everyday story in six frames."},
		{"06", "Make a personal project", "Choose a manageable idea, define what finished means and share a small result. Exercise: plan three sessions with one clear outcome each."},
	}},
}

//go:embed telemt_web_themed.html
var telemtWebThemedHTML string

//go:embed telemt_web_media.html
var telemtWebMediaHTML string
var telemtWebMediaTemplate = template.Must(template.New("media").Parse(telemtWebMediaHTML))

var telemtWebThemedTemplate = template.Must(template.New("cover").Parse(telemtWebThemedHTML))

func isTelemtWebTheme(name string) bool {
	switch name {
	case "notes", "technology", "travel", "nature":
		return true
	}
	_, ok := telemtWebThemes[name]
	return ok
}

func renderTelemtWebTheme(domain string, site *TelemtWebSiteOptions, theme telemtWebTheme) (string, error) {
	if strings.TrimSpace(site.Title) != "" {
		theme.Title = site.Title
	}
	if strings.TrimSpace(site.Description) != "" {
		theme.Description = site.Description
	}
	var page bytes.Buffer
	renderer := telemtWebThemedTemplate
	if theme.Media {
		renderer = telemtWebMediaTemplate
	}
	err := renderer.Execute(&page, struct {
		telemtWebTheme
		Domain string
	}{theme, domain})
	return page.String(), err
}
