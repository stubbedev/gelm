// Package emoji is gelm's emoji chooser data (#92): a curated table
// of common Unicode emoji grouped by category with search keywords,
// parsed once from a compact literal. This is a starter table - a few
// hundred well-chosen emoji, not the full CLDR set - kept in one
// place so a bigger table is a data change, not a code change.
package emoji

import (
	"strings"
)

// Emoji is one table entry: the emoji itself, its name, and search
// keywords (name included).
type Emoji struct {
	Glyph    string
	Name     string
	Keywords []string
}

// Category groups emoji under the chooser's section headers, in table
// order.
type Category struct {
	Name  string
	Items []Emoji
}

// table is the literal: category headers as "## name" lines, entries
// as "glyph name | keyword keyword" lines.
const table = `
## Smileys & Emotion
😀 grinning face | happy smile joy
😃 smiling face | happy smile joy
😄 smiling eyes | happy laugh smile
😁 beaming face | grin happy smile
😆 squinting laugh | laugh happy haha
😅 sweat smile | laugh nervous relief
🤣 rolling laugh | rofl laugh haha
😂 joy tears | cry laugh happy
🙂 slight smile | happy small smile
🙃 upside down | silly irony
😉 wink | hint smile
😊 blush smile | happy warm smile
😇 innocent halo | angel halo
🥰 smiling hearts | love adore hearts
😍 heart eyes | love like adore
🤩 star struck | wow amazing stars
😘 kiss | love heart
😗 kissing | kiss
😚 kiss closed eyes | kiss
😙 kiss smiling eyes | kiss
🥲 tear smile | touched happy sad
😋 yum | tasty delicious
😛 tongue | silly playful
😜 winking tongue | silly playful zany
🤪 zany | crazy silly
😝 squinting tongue | silly
🤑 money mouth | rich greedy
🤗 hug | care comfort
🤭 hand mouth | oops giggle
🤫 shush | quiet secret
🤔 thinking | hmm consider
🤐 zipper mouth | silence secret
🤨 raised eyebrow | suspicious really
😐 neutral face | meh blank
😑 expressionless | blank meh
😶 no mouth | silent blank
😏 smirk | smug sly
😒 unamused | sad bored meh
🙄 rolling eyes | annoyance whatever
😬 grimacing | awkward teeth
🤥 lying nose | liar pinocchio
😌 relieved | peaceful calm
😔 pensive | sad thoughtful
😪 sleepy | tired bed
🤤 drooling | sleepy want
😴 sleeping | tired zzz snore
😷 mask | sick health ill
🤒 thermometer | sick fever
🤕 bandage | hurt injured
🤢 nauseated | sick green
🤮 vomiting | sick puke
🤧 sneeze | sick achoo
🥵 hot | heat sweating
🥶 cold | freezing winter
🥴 woozy | drunk dizzy
😵 dizzy | dead crossed eyes
🤯 exploding head | mind blown shock
🤠 cowboy | western hat
🥳 party | celebrate fun
🥸 disguised | incognito
😎 sunglasses | cool chill
🤓 nerd | glasses smart
🧐 monocle | inspect classy
😕 confused | puzzled
😟 worried | sad concerned
🙁 slight frown | sad unhappy
😮 open mouth | wow surprise
😯 hushed | surprise quiet
😲 astonished | shock wow
😳 flushed | shy embarrassed
🥺 pleading | puppy beg
😦 frowning | sad
😧 anguished | distress
😨 fearful | scared afraid
😰 anxious | sad nervous sweat
😥 sad relieved | cry
😢 crying | sad tear cry
😭 loudly crying | sob sad bawl
😱 screaming | fear shock
😖 confounded | frustrated
😫 tired | weary exhausted
🥱 yawning | tired sleepy bored
😤 triumph | huff steam
😡 rage | angry mad red
😠 angry | mad upset
🤬 cursing | swear angry
😈 devil | evil imp
💀 skull | dead
💩 poop | funny
🤡 clown | circus creepy
👻 ghost | spooky halloween
👽 alien | ufo space
🤖 robot | bot machine
😺 smiling cat | kitty happy
😸 grinning cat | kitty
😹 cat joy tears | kitty laugh
😻 cat heart eyes | kitty love
😼 cat smirk | kitty
😽 cat kiss | kitty
🙀 cat scream | kitty shock
😿 cat cry | kitty sad
😾 cat pouting | kitty mad

## People & Body
👋 waving hand | hello hi bye
🤚 raised back of hand | stop
🖐️ hand fingers splayed | five
✋ raised hand | stop high five
🖖 vulcan | spock star trek
👌 ok | okay perfect
🤌 pinched fingers | italian chef
🤏 pinch | small tiny
✌️ victory peace | peace win
🤞 crossed fingers | luck hope
🤟 love you | ilu
🤘 rock on | horns metal
🤙 call me | shaka surf
👈 pointing left | left
👉 pointing right | right
👆 pointing up | up
👇 pointing down | down
👍 thumbs up | yes like approve good
👎 thumbs down | no dislike bad
✊ fist bump | power punch
👊 punch | fist bro
🤛 left fist | bump
🤜 right fist | bump
👏 clapping | applause yes bravo
🙌 raising hands | hooray celebrate
👐 open hands | hug
🤲 palms up | offer pray
🤝 handshake | deal agreement
🙏 pray | thanks please namaste
✍️ writing | note
💪 muscle | strong flex arm
🦾 mechanical arm | robot prosthetic
🦵 leg | limb
🦶 foot | stomp
👂 ear | listen
👃 nose | smell
🧠 brain | smart mind
🦷 tooth | dentist
👀 eyes | look watch see
👁️ eye | look watch
👅 tongue | taste
👄 lips | mouth kiss
🫶 heart hands | love care

## Animals & Nature
🐶 dog face | puppy pet woof
🐱 cat face | kitty pet meow
🐭 mouse face | rat
🐹 hamster | pet
🐰 rabbit face | bunny easter
🦊 fox | clever
🐻 bear | grizzly
🐼 panda | china bamboo
🐨 koala | australia
🐯 tiger face | roar
🦁 lion | roar king
🐮 cow face | moo
🐷 pig face | oink
🐸 frog | toad
🐵 monkey face | ape
🙈 see no evil | monkey shy
🙉 hear no evil | monkey
🙊 speak no evil | monkey
💥 explosion | boom bang
💫 dizzy | star sparkle
💦 sweat droplets | water splash
💨 dash | wind fast
🕳️ hole | gap
🍌 banana | fruit food
🍎 red apple | fruit food
🍏 green apple | fruit
🍐 pear | fruit
🍊 tangerine | orange fruit
🍋 lemon | citrus sour
🍉 watermelon | fruit summer
🍇 grapes | fruit wine
🍓 strawberry | berry fruit
🥝 kiwi | fruit
🍅 tomato | fruit vegetable
🥕 carrot | vegetable rabbit
🌽 corn | maize vegetable
🌶️ hot pepper | spicy chili
🥔 potato | spud
🍞 bread | toast loaf
🧀 cheese | cheddar wedge
🍗 poultry leg | chicken meat
🍔 hamburger | burger fast food
🍟 fries | chips fast food
🍕 pizza | slice italian
🌭 hot dog | sausage frank
🥪 sandwich | sub
🌮 taco | mexican
🌯 burrito | mexican wrap
🥗 salad | greens healthy
🍝 spaghetti | pasta italian
🍜 ramen | noodles soup
🍣 sushi | japanese fish
🍤 shrimp | prawn tempura
🍚 rice | bowl
🍦 ice cream | dessert soft serve
🍩 doughnut | donut dessert
🍪 cookie | biscuit dessert
🎂 birthday cake | party candle
🍰 cake slice | dessert shortcake
🍫 chocolate | candy dessert
🍿 popcorn | movie cinema
☕ coffee | cafe espresso hot
🍵 tea | green matcha
🧊 ice cube | cold frozen
⚽ soccer | football ball
🏀 basketball | ball hoop
🏈 american football | ball
⚾ baseball | ball
🎾 tennis | ball racket
🏐 volleyball | ball beach
🎱 8 ball | pool billiards
🏓 ping pong | table tennis paddle
🏸 badminton | shuttle racket
🥊 boxing glove | fight punch
🎯 dart | bullseye target
🎮 game controller | video game pad
🕹️ joystick | arcade game
🎲 dice | random game
🎸 guitar | rock music
🎹 piano | keyboard music
🎺 trumpet | brass music
🎻 violin | strings music
🥁 drum | percussion music
🎤 microphone | sing karaoke
🎧 headphones | music listen audio
🎨 art | paint palette
🧩 puzzle | piece jigsaw

## Travel & Places
🚗 car | auto drive
🚕 taxi | cab
🚌 bus | transit
🏎️ race car | formula speed
🚓 police car | cop
🚑 ambulance | emergency medic
🚒 fire engine | firefighter
🚚 truck | delivery lorry
🚜 tractor | farm
🛴 scooter | kick
🚲 bicycle | bike cycle
🏍️ motorcycle | motorbike
✈️ airplane | flight travel
🚀 rocket | launch space
🛸 ufo | alien saucer
🚁 helicopter | chopper
⛵ sailboat | yacht sea
🚢 ship | ferry cargo
🗺️ world map | travel atlas
🏔️ snowy mountain | peak alps
⛰️ mountain | peak
🌋 volcano | erupt lava
🏕️ camping | tent outdoors
🏖️ beach | seaside sand vacation
🏜️ desert | sand dunes
🏝️ island | tropical deserted
🌇 sunset | city dusk
🌆 cityscape | dusk city
🌃 night | stars city dark
🌉 bridge | night golden gate
🎡 ferris wheel | carnival fair
🎢 roller coaster | theme park
🎪 circus | tent big top
🗼 tokyo tower | japan landmark
🗽 statue of liberty | new york landmark
🏯 castle japan | shiro
🏟️ stadium | sports arena
🏰 castle | medieval palace
🌈 rainbow | colors pride
☀️ sun | sunny day hot
🌤️ sun behind cloud | partly cloudy
⛅ partly cloudy | sun cloud
☁️ cloud | cloudy sky
🌧️ rain | rainy weather
⛈️ thunderstorm | lightning rain
❄️ snowflake | snow cold winter
☃️ snowman | winter cold
🌊 wave | water ocean sea
🌙 crescent moon | night sleep
🌟 glowing star | sparkle shine
✨ sparkles | magic shine clean
☄️ comet | shooting star space
🔥 fire | hot flame lit
💧 droplet | water
⭐ star | favorite night

## Objects & Symbols
⌚ watch | clock wrist
📱 phone | mobile cell smartphone
💻 laptop | computer macbook
⌨️ keyboard | computer type
🖥️ desktop | computer monitor
🖨️ printer | office paper
🖱️ mouse | computer click
💽 floppy | disk retro save
💾 save | floppy disk
💿 cd | disc optical
📀 dvd | disc
📷 camera | photo picture
📸 camera flash | photo
📹 video camera | record film
📺 tv | television screen
📻 radio | broadcast
⏰ alarm | clock morning wake
⏳ hourglass | time wait sand
🕰️ clock | time old
🔋 battery | power charge
💡 bulb | idea light
🔦 flashlight | torch
🕯️ candle | light flame
🧯 extinguisher | fire safety
🗑️ wastebasket | trash bin delete
🔎 search | magnify find
🔒 locked | secure private
🔓 unlocked | open
🔑 key | unlock password
🔨 hammer | tool build
🪛 screwdriver | tool
🔧 wrench | tool fix settings
⚙️ gear | settings config
🧰 toolbox | tools
🧲 magnet | attract
💉 syringe | shot vaccine medicine
💊 pill | medicine drug
🚪 door | enter exit
🛏️ bed | sleep hotel
🛋️ couch | sofa lounge
🚿 shower | bath clean
🧼 soap | wash clean
🧹 broom | sweep clean
🧺 basket | laundry
🛒 cart | shopping grocery
🎁 gift | present birthday
🎈 balloon | party
🎉 party popper | celebrate tada confetti
🎊 confetti | celebrate ball
🎀 ribbon | bow decoration
❤️ red heart | love like heart
🧡 orange heart | love
💛 yellow heart | love
💚 green heart | love
💙 blue heart | love
💜 purple heart | love
🖤 black heart | love dark
🤍 white heart | love
💔 broken heart | sad heartbreak
❤️‍🔥 heart fire | intense love
💯 hundred | perfect score hundred
💢 anger | mad red symbol
💬 speech | chat message talk
💭 thought | think dream
💤 zzz | sleep tired
⏸️ pause | media control
▶️ play | media control start
🔁 repeat | loop media
🔀 shuffle | random media
➕ plus | add math
➖ minus | remove math
✖️ multiply | math times
➗ divide | math
✅ check | done ok complete yes
❌ cross | no wrong error
⚠️ warning | caution alert
🚫 prohibited | no forbidden
♻️ recycle | green eco
🔱 trident | poseidon three
⚜️ fleur | royal
🔔 bell | notification ring alert
🔕 bell off | mute silent
🎵 music note | song tune
🎶 music notes | song melody
✔️ check mark | done yes
❗ exclamation | important alert
❓ question | help unknown
‼️ double exclamation | important
⁉️ interrobang | question exclaim
🔃 arrows clock | refresh cycle sync
🕐 one oclock | time clock
`

// Categories is the parsed table, in order.
var Categories = parse(table)

// All flattens the table.
func All() []Emoji {
	var out []Emoji
	for _, c := range Categories {
		out = append(out, c.Items...)
	}
	return out
}

// Matches returns emoji whose name or keywords contain every word of
// the query (case-insensitive), capped at n, in table order.
func Matches(query string, n int) []Emoji {
	words := strings.Fields(strings.ToLower(strings.TrimSpace(query)))
	if len(words) == 0 {
		return nil
	}
	var out []Emoji
	for _, e := range All() {
		hay := strings.ToLower(e.Name + " " + strings.Join(e.Keywords, " "))
		ok := true
		for _, w := range words {
			if !strings.Contains(hay, w) {
				ok = false
				break
			}
		}
		if ok {
			out = append(out, e)
			if n > 0 && len(out) >= n {
				break
			}
		}
	}
	return out
}

// parse reads the literal table.
func parse(src string) []Category {
	var cats []Category
	for line := range strings.SplitSeq(src, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "":
			// Blank lines separate; they create nothing.
		case strings.HasPrefix(line, "## "):
			cats = append(cats, Category{Name: strings.TrimPrefix(line, "## ")})
		case strings.Contains(line, " | "):
			left, keywords, _ := strings.Cut(line, " | ")
			// The glyph is the leading emoji run (every non-ASCII byte);
			// the words after it are the name.
			cut := strings.IndexFunc(left, func(r rune) bool { return r == ' ' })
			if cut < 0 || len(cats) == 0 {
				continue
			}
			cats[len(cats)-1].Items = append(cats[len(cats)-1].Items, Emoji{
				Glyph:    left[:cut],
				Name:     strings.TrimSpace(left[cut:]),
				Keywords: strings.Fields(strings.ToLower(keywords)),
			})
		}
	}
	if n := len(cats); n > 0 && cats[n-1].Name == "" && len(cats[n-1].Items) == 0 {
		cats = cats[:n-1]
	}
	return cats
}
