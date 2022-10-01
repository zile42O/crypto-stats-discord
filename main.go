package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/fatih/color"
	_ "github.com/go-sql-driver/mysql"
)

var (
	config                = new(Configuration)
	countGuild        int = 0
	ServersGuilds         = make(map[int]string)
	activeCommands        = make(map[string]command)
	disabledCommands      = make(map[string]command)
	CooldownCMD           = make(map[string]int64)
	footer                = new(discordgo.MessageEmbedFooter)
	mem               runtime.MemStats
	Guild_Btc_Channel         = make(map[string]string)
	Guild_Eth_Channel         = make(map[string]string)
	lastBTC           float64 = 0
	lastETH           float64 = 0
	btc_str           string  = "btc price"
	ltc_str           string  = "btc price"
	eth_str           string  = "eth price"
	eth_lasttime      int64   = 0
	btc_lasttime      int64   = 0
	MYSQL_CONNECT_URL string  = "user:password?@tcp(host:3306)/tab"
)

type Configuration struct {
	Game    string `json:"game"`
	Prefix  string `json:"prefix"`
	Token   string `json:"token"`
	OwnerID string `json:"owner_id"`
	MaxProc int    `json:"maxproc"`
}

type command struct {
	Name string
	Help string

	OwnerOnly     bool
	RequiresPerms bool

	PermsRequired int64

	Exec func(*discordgo.Session, *discordgo.MessageCreate, []string)
}

type CryptoStats_SqlStructure struct {
	id  string `json:"user_id"`
	BTC string `json:"btc_channel"`
	ETH string `json:"eth_channel"`
}
type coinsData struct {
	Symbol            string `json:"symbol"`
	Price             string `json:"price"`
	Name              string `json:"name"`
	High              string `json:"high"`
	Status            string `json:"status"`
	Rank              string `json:"rank"`
	CirculatingSupply string `json:"circulating_supply"`
	MaxSupply         string `json:"max_supply"`
	Marketcap         string `json:"market_cap"`
	FirstCandle       string `json:"first_candle"`
	FirstTrade        string `json:"first_trade"`
}

func main() {
	loadConfig()

	db, err := sql.Open("mysql", MYSQL_CONNECT_URL)
	if err != nil {
		errorln("Failed MySQL connection > %s", err.Error())
		return
	}
	color.Green("MySQL successfully initalized!")

	runtime.GOMAXPROCS(config.MaxProc)
	session, err := discordgo.New(fmt.Sprintf("Bot %s", string(config.Token)))
	if err != nil {
		errorln("Failed create Discord session err: %s", err)
		return
	}
	session.Identify.Intents = discordgo.MakeIntent(discordgo.IntentsGuildMembers | discordgo.IntentsAllWithoutPrivileged)
	_, err = session.User("@me")
	if err != nil {
		errorln("Session @me err: %s", err)
	}
	// E V E N T S
	session.AddHandler(guildJoin)
	session.AddHandler(guildLeave)
	session.AddHandler(messageCreate)
	//MySQL
	results, err := db.Query("SELECT id, btc_channel, eth_channel FROM `guilds`")
	if err == nil {
		for results.Next() {
			var Server CryptoStats_SqlStructure
			err = results.Scan(&Server.id, &Server.BTC, &Server.ETH)
			if err == nil {
				Guild_Btc_Channel[Server.id] = Server.BTC
				Guild_Eth_Channel[Server.id] = Server.ETH
			} else {
				errorln("MySQL scan results: ", err)
			}
		}
		color.Green("Loaded guilds from database!")
	} else {
		errorln("MySQL results: ", err)
	}
	db.Close()
	//
	err = session.Open()
	if err != nil {
		errorln("Failed opening connection err: %s", err)
		return
	}

	color.Green("Bot is now running. Press CTRL-C to exit.")
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGINT, syscall.SIGTERM, os.Interrupt, os.Kill)
	<-sc
	session.Close()
}

// T I M E R / U P D A T E R

func get_BtcPrice(guildID string) float64 {
	url := "https://api.nomics.com/v1/currencies/ticker?key=9bb9a6f4dfcd83c6541d871b90520475450365a8&ids=BTC&interval=1h&convert=EUR&per-page=100&page=1"
	res, err := http.Get(url)
	if err != nil {
		return lastBTC
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return lastBTC
	}
	var data []coinsData
	err = json.Unmarshal(body, &data)
	if err == nil {
		for _, values := range data {
			if values.Symbol == "BTC" {
				if s, err := strconv.ParseFloat(string(values.Price), 32); err == nil {
					return s
				}
			}
		}
	}
	return lastBTC
}

func get_EthPrice(guildID string) float64 {
	url := "https://api.nomics.com/v1/currencies/ticker?key=9bb9a6f4dfcd83c6541d871b90520475450365a8&ids=ETH&interval=1h&convert=EUR&per-page=100&page=1"
	res, err := http.Get(url)
	if err != nil {
		return lastETH
	}
	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return lastETH
	}
	var data []coinsData
	err = json.Unmarshal(body, &data)
	if err == nil {
		for _, values := range data {
			if values.Symbol == "ETH" {
				if s, err := strconv.ParseFloat(string(values.Price), 32); err == nil {
					return s
				}
			}
		}
	}
	return lastETH
}

func SetupTimerStats(s *discordgo.Session, serverID string) {
	if serverID != "" {
		color.Blue("SetupTimerStats called for serverID: %s", serverID)
		for _ = range time.Tick(time.Minute * 5) {
			t := time.Now().Unix()
			if t > btc_lasttime {
				color.Blue("BTC API called")
				url := "https://api.nomics.com/v1/currencies/ticker?key=9bb9a6f4dfcd83c6541d871b90520475450365a8&ids=BTC&interval=1h&convert=EUR&per-page=100&page=1"
				res, err := http.Get(url)
				if err != nil {
					errorln("Can't get price for BTC err: %s", err)
					return
				}
				defer res.Body.Close()

				body, err := ioutil.ReadAll(res.Body)
				if err != nil {
					errorln("Can't read body price for BTC err: %s", err)
					return
				}
				var data []coinsData
				err = json.Unmarshal(body, &data)
				if err == nil {
					var newbtc float64
					for _, values := range data {
						if values.Symbol == "BTC" {
							if s, err := strconv.ParseFloat(string(values.Price), 32); err == nil {
								newbtc = s
								break
							}
						}
					}
					if newbtc > lastBTC {
						btc_str = fmt.Sprintf("📈 BTC: %.2f EUR", newbtc)
					} else {
						btc_str = fmt.Sprintf("📉 BTC: %.2f EUR", newbtc)
					}
					lastBTC = newbtc
				}
			} else {
				btc_lasttime = t + 2
			}
			color.Yellow("BTC Updated!")
			s.ChannelEdit(Guild_Btc_Channel[serverID], btc_str)
			time.Sleep(time.Second * 1)
			t = time.Now().Unix()
			if t > eth_lasttime {
				color.Blue("ETH API called")
				url := "https://api.nomics.com/v1/currencies/ticker?key=9bb9a6f4dfcd83c6541d871b90520475450365a8&ids=ETH&interval=1h&convert=EUR&per-page=100&page=1"
				res, err := http.Get(url)
				if err != nil {
					errorln("Can't get price for ETH err: %s", err)
				}
				defer res.Body.Close()

				body, err := ioutil.ReadAll(res.Body)
				if err != nil {
					errorln("Can't read body price for ETH err: %s", err)
				}
				var data []coinsData
				err = json.Unmarshal(body, &data)
				if err == nil {
					var neweth float64
					for _, values := range data {
						if values.Symbol == "ETH" {
							if s, err := strconv.ParseFloat(string(values.Price), 32); err == nil {
								neweth = s
								break
							}
						}
					}
					if neweth > lastETH {
						eth_str = fmt.Sprintf("📈 ETH: %.2f EUR", neweth)
					} else {
						eth_str = fmt.Sprintf("📉 ETH: %.2f EUR", neweth)
					}
					lastETH = neweth
				}
			} else {
				eth_lasttime = t + 2
			}
			color.Yellow("ETH Updated!")
			s.ChannelEdit(Guild_Eth_Channel[serverID], eth_str)
		}
	}
}

// I N I T

func init() {
	footer.Text = "Last Update: 3/12/2022\nLast Bot reboot: " + time.Now().Format("2006-01-02 3:4:5 pm")
	newCommand("help", 0, false, cmdHelp).add()
	newCommand("info", 0, false, cmdInfo).add()
	newCommand("crypto", 0, false, cmdCrypto).setHelp("$crypto 'currency' (Get stats info about currency)\nExample: $crypto BTC").add()
	newCommand("setup", 0, false, cmdSetup).add().ownerOnly()
}

// E V E N T S
func messageCreate(s *discordgo.Session, m *discordgo.MessageCreate) {
	if m.Author.Bot {
		return
	}

	repeatblock := strings.Count(m.Content, config.Prefix)
	if repeatblock > 1 {
		return
	}
	//End fix

	guildDetails, err := guildDetails(m.ChannelID, "", s)
	if err != nil {
		return
	}

	prefix, err := activePrefix(m.ChannelID, s)
	if err != nil {
		return
	}

	if !strings.HasPrefix(m.Content, config.Prefix) && !strings.HasPrefix(m.Content, prefix) {
		return
	}
	parseCommand(s, m, guildDetails, func() string {
		if strings.HasPrefix(m.Content, config.Prefix) {
			return strings.TrimPrefix(m.Content, config.Prefix)
		}
		return strings.TrimPrefix(m.Content, prefix)
	}())
}

func guildJoin(s *discordgo.Session, m *discordgo.GuildCreate) {
	if m.Unavailable {
		errorln("Unavailable to join guild %s", m.Guild.ID)
		return
	}
	color.Yellow("Joined to server id: %s name: %s", m.Guild.ID, m.Guild.Name)
	lastBTC = get_BtcPrice(m.Guild.ID)
	time.Sleep(time.Second * 1)
	lastETH = get_EthPrice(m.Guild.ID)
	btc_str = fmt.Sprintf("BTC: %.2f EUR", lastBTC)
	eth_str = fmt.Sprintf("ETH: %.2f EUR", lastETH)
	color.Green("Btc for server ID: %s Load: %s", m.Guild.ID, btc_str)
	color.Green("Eth for server ID: %s Load: %s", m.Guild.ID, eth_str)
	countGuild++
	if err := s.UpdateGameStatus(0, fmt.Sprintf("CryptoStats | $help | Servers: %d", countGuild)); err != nil {
		color.Red("Can't set bot game status, error: %s", err)
		return
	}
	SetupTimerStats(s, m.ID)
}

func guildLeave(s *discordgo.Session, m *discordgo.GuildDelete) {
	if m.Unavailable {
		guild, err := guildDetails("", m.Guild.ID, s)
		if err != nil {
			errorln("Unavailable guild id: %s", m.Guild.ID)
			return
		}
		errorln("Unavailable guild id: %s name: %s", m.Guild.ID, guild.Name)
		return
	}
	countGuild--
	if err := s.UpdateGameStatus(0, fmt.Sprintf("CryptoStats | $help | Servers: %d", countGuild)); err != nil {
		color.Red("Can't set bot game status, error: %s", err)
		return
	}
	color.Yellow("Leaved guild id: %s name: %s", m.Guild.ID, m.Name)
	db, _ := sql.Open("mysql", MYSQL_CONNECT_URL)
	_, _ = db.Query(fmt.Sprintf("DELETE FROM `guilds` WHERE `id`=%s", m.ID))
}

// F U N C T I O N S

func errorln(format string, a ...interface{}) {
	str := "Error: "
	str += format
	color.Red(str, a...)
	file, err := os.OpenFile("errors.log", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)
	if err != nil {
		log.Fatal(err)
	}
	log.SetOutput(file)
	log.Printf(str, a...)
	defer file.Close()
}

func guildDetails(channelID, guildID string, s *discordgo.Session) (guildDetails *discordgo.Guild, err error) {
	if guildID == "" {
		var channel *discordgo.Channel
		channel, err = channelDetails(channelID, s)
		if err != nil {
			return
		}
		guildID = channel.GuildID
	}

	guildDetails, err = s.State.Guild(guildID)
	if err != nil {
		if err == discordgo.ErrStateNotFound {
			guildDetails, err = s.Guild(guildID)
			if err != nil {
				errorln("Getting guild details =>", guildID, err)
			}
		}
	}
	return
}

func channelDetails(channelID string, s *discordgo.Session) (channelDetails *discordgo.Channel, err error) {
	channelDetails, err = s.State.Channel(channelID)
	if err != nil {
		if err == discordgo.ErrStateNotFound {
			channelDetails, err = s.Channel(channelID)
			if err != nil {
				errorln("Getting channel details =>", channelID, err)
			}
		}
	}
	return
}

func permissionDetails(authorID, channelID string, s *discordgo.Session) (userPerms int64, err error) {
	userPerms, err = s.State.UserChannelPermissions(authorID, channelID)
	if err != nil {
		if err == discordgo.ErrStateNotFound {
			userPerms, err = s.UserChannelPermissions(authorID, channelID)
			if err != nil {
				errorln("Getting permission details err: %s", err)
			}
		}
	}
	return
}

// Config

func loadJSON(path string, v interface{}) error {
	f, err := os.OpenFile(path, os.O_RDONLY, 0600)
	if err != nil {
		errorln("Loading Err > Path: %s Err: %s", path, err)
		return err
	}

	if err := json.NewDecoder(f).Decode(v); err != nil {
		errorln("Loading Err > Path: %s Err: %s", path, err)
		return err
	}
	return nil
}
func saveJSON(path string, data interface{}) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		errorln("Saving Err > Path: %s Err: %s", path, err)
		return err
	}

	if err = json.NewEncoder(f).Encode(data); err != nil {
		errorln("Saving Err > Path: %s Err: %s", path, err)
		return err
	}
	return nil
}

func loadConfig() error {
	return loadJSON("config.json", config)
}

func saveConfig() error {
	return saveJSON("config.json", config)
}

//Commands

func newCommand(name string, permissions int64, needsPerms bool, f func(*discordgo.Session, *discordgo.MessageCreate, []string)) command {
	return command{
		Name:          name,
		PermsRequired: permissions,
		RequiresPerms: needsPerms,
		Exec:          f,
	}
}
func (c command) alias(a string) command {
	activeCommands[strings.ToLower(a)] = c
	return c
}

func (c command) setHelp(help string) command {
	c.Help = help
	return c
}

func (c command) ownerOnly() command {
	c.OwnerOnly = true
	return c
}
func parseCommand(s *discordgo.Session, m *discordgo.MessageCreate, guildDetails *discordgo.Guild, message string) {
	msglist := strings.Fields(message)
	if len(msglist) == 0 {
		return
	}
	t := time.Now().Unix()
	if t < CooldownCMD[m.Author.ID] {
		s.ChannelMessageSendEmbed(m.ChannelID, &discordgo.MessageEmbed{
			Color:       0xad2e2e,
			Description: fmt.Sprintf("You calling commands so fast, please slow down!"),
			Footer:      footer,
			Fields: []*discordgo.MessageEmbedField{
				{Name: "Required wait time:", Value: "3 seconds", Inline: false},
			},
		})
		return
	} else {
		CooldownCMD[m.Author.ID] = t + 3 //+3 sec wait
	}
	isOwner := m.Author.ID == config.OwnerID
	commandName := strings.ToLower(func() string {
		if strings.HasPrefix(message, " ") {
			return " " + msglist[0]
		}
		return msglist[0]
	}())

	color.Blue(fmt.Sprintf("(debug): Author ID: %s, Author Username: %s#%s, Guild ID: %s, Server: %s, Command: %s", m.Author.ID, m.Author.Username, m.Author.Discriminator, guildDetails.ID, guildDetails.Name, m.Content))

	if command, ok := activeCommands[commandName]; ok && commandName == strings.ToLower(command.Name) {
		userPerms, err := permissionDetails(m.Author.ID, m.ChannelID, s)
		if err != nil {
			s.ChannelMessageSendEmbed(m.ChannelID, &discordgo.MessageEmbed{
				Color:       0xad2e2e,
				Description: fmt.Sprintf("Can't parse permissions!"),
				Footer:      footer,
			})
			return
		}

		hasPerms := userPerms&command.PermsRequired > 0
		if (!command.OwnerOnly && !command.RequiresPerms) || (command.RequiresPerms && hasPerms) || isOwner {
			command.Exec(s, m, msglist)
			return
		}
		s.ChannelMessageSendEmbed(m.ChannelID, &discordgo.MessageEmbed{
			Color:       0xad2e2e,
			Description: fmt.Sprintf("You don't have permissions for this command!"),
			Footer:      footer,
			Fields: []*discordgo.MessageEmbedField{
				{Name: "Required roles:", Value: "👑Ownership", Inline: false},
			},
		})
		return
	} else {
		s.ChannelMessageSendEmbed(m.ChannelID, &discordgo.MessageEmbed{
			Color:       0xad2e2e,
			Description: fmt.Sprintf("Unknown command."),
			Footer:      footer,
			Fields: []*discordgo.MessageEmbedField{
				{Name: "Check commands:", Value: config.Prefix + "help", Inline: false},
			},
		})
		return
	}
	activeCommands["bigmoji"].Exec(s, m, msglist)
}

func activePrefix(channelID string, s *discordgo.Session) (prefix string, err error) {
	prefix = config.Prefix
	_, err = guildDetails(channelID, "", s)
	if err != nil {
		s.ChannelMessageSend(channelID, "There was an issue executing the command :( Try again please~")
		return
	}
	return prefix, nil
}

func (c command) add() command {
	activeCommands[strings.ToLower(c.Name)] = c
	return c
}

func codeBlock(s ...string) string {
	return "```" + strings.Join(s, " ") + "```"
}

// C O M M A N D S

func cmdSetup(s *discordgo.Session, m *discordgo.MessageCreate, msglist []string) {
	s.ChannelTyping(m.ChannelID)
	ch, err := s.GuildChannelCreate(m.GuildID, fmt.Sprintf("📈 BTC: %.2f EUR", get_BtcPrice(m.GuildID)), discordgo.ChannelTypeGuildVoice)
	if err == nil {
		s.ChannelMessageSendEmbed(m.ChannelID, &discordgo.MessageEmbed{
			Color:       0x7cad2e,
			Description: fmt.Sprintf("BTC successfully setup :ok_hand:, now you can edit channels position and permisions"),
			Footer:      footer,
		})
		Guild_Btc_Channel[m.GuildID] = ch.ID
	}
	s.ChannelTyping(m.ChannelID)
	time.Sleep(time.Second * 2)
	s.ChannelTyping(m.ChannelID)
	ch, err = s.GuildChannelCreate(m.GuildID, fmt.Sprintf("📈 ETH: %.2f EUR", get_EthPrice(m.GuildID)), discordgo.ChannelTypeGuildVoice)
	if err == nil {
		s.ChannelMessageSendEmbed(m.ChannelID, &discordgo.MessageEmbed{
			Color:       0x7cad2e,
			Description: fmt.Sprintf("ETH successfully setup :ok_hand:, now you can edit channels position and permisions"),
			Footer:      footer,
		})
		Guild_Eth_Channel[m.GuildID] = ch.ID
	}
	s.ChannelTyping(m.ChannelID)
	db, err := sql.Open("mysql", MYSQL_CONNECT_URL)
	if err != nil {
		errorln("Failed MySQL connection > %s", err.Error())
		s.ChannelMessageSendEmbed(m.ChannelID, &discordgo.MessageEmbed{
			Color:       0x7cad2e,
			Description: fmt.Sprintf("Failed to insert it in database, try again!"),
			Footer:      footer,
		})
		return
	}
	_, err = db.Query(fmt.Sprintf("DELETE FROM `guilds` WHERE `id`=%s", m.GuildID))

	_, err = db.Query(fmt.Sprintf("INSERT INTO `guilds` (`id`, `btc_channel`, `eth_channel`) VALUES (%s,%s,%s)", m.GuildID, Guild_Btc_Channel[m.GuildID], Guild_Eth_Channel[m.GuildID]))

	if err == nil {
		color.Yellow("Added guild id: %s in database", m.GuildID)
	}
	s.ChannelMessageSendEmbed(m.ChannelID, &discordgo.MessageEmbed{
		Color:       0x7cad2e,
		Description: fmt.Sprintf("Done! :ok_hand:"),
		Footer:      footer,
	})
}

func getCreationTime(ID string) (t time.Time, err error) {
	i, err := strconv.ParseInt(ID, 10, 64)
	if err != nil {
		return
	}

	timestamp := (i >> 22) + 1420070400000
	t = time.Unix(timestamp/1000, 0)
	return
}

func cmdInfo(s *discordgo.Session, m *discordgo.MessageCreate, msglist []string) {
	s.ChannelTyping(m.ChannelID)
	ct1, _ := getCreationTime(s.State.User.ID)
	creationTime := ct1.Format("2006-01-02 3:4:5 pm")

	runtime.ReadMemStats(&mem)
	s.ChannelMessageSendEmbed(m.ChannelID, &discordgo.MessageEmbed{
		Color: 0x00ff00,
		Title: "📊・Crypto Stats Info",
		Fields: []*discordgo.MessageEmbedField{
			{Name: "Bot Name:", Value: codeBlock(s.State.User.Username), Inline: true},
			{Name: "Creator:", Value: codeBlock("Zile42O#0420"), Inline: true},
			{Name: "Creation Date:", Value: codeBlock(creationTime), Inline: true},
			{Name: "Global Prefix:", Value: codeBlock(config.Prefix), Inline: true},
			{Name: "Programming Language:", Value: codeBlock("Go"), Inline: true},
			{Name: "Library:", Value: codeBlock("DiscordGo"), Inline: true},
			{Name: "Guilds (Servers):", Value: codeBlock(fmt.Sprint("", countGuild)), Inline: true},
			{Name: "Memory Usage:", Value: codeBlock(strconv.Itoa(int(mem.Alloc/1024/1024)) + "MB"), Inline: true},
			{Name: "42O's discord:", Value: "discord.gg/42o or discord.420-clan.com"},
		},
	})
}

func (c command) helpCommand(s *discordgo.Session, m *discordgo.MessageCreate) {
	s.ChannelMessageSendEmbed(m.ChannelID, &discordgo.MessageEmbed{
		Color: 0x00ff00,

		Fields: []*discordgo.MessageEmbedField{
			{
				Name:  c.Name,
				Value: c.Help,
			},
		},

		Footer: footer,
	})
}

func cmdHelp(s *discordgo.Session, m *discordgo.MessageCreate, msglist []string) {
	s.ChannelTyping(m.ChannelID)
	if len(msglist) == 2 {
		if val, ok := activeCommands[strings.ToLower(msglist[1])]; ok {
			val.helpCommand(s, m)
			return
		}
	}

	var commands []string
	for _, val := range activeCommands {
		if m.Author.ID == config.OwnerID || !val.OwnerOnly {
			commands = append(commands, "`"+val.Name+"`")
		}
	}

	prefix := config.Prefix
	s.ChannelMessageSendEmbed(m.ChannelID, &discordgo.MessageEmbed{
		Color: 0x00ff00,
		Fields: []*discordgo.MessageEmbedField{
			{Name: "CryptoStats Bot Help", Value: strings.Join(commands, ", ") + "\n\nUse `" + prefix + "help [command]` for detailed info about a command."},
		},
	})
}
func cmdCrypto(s *discordgo.Session, m *discordgo.MessageCreate, msglist []string) {
	if len(msglist) < 2 {
		s.ChannelMessageSendEmbed(m.ChannelID, &discordgo.MessageEmbed{
			Color:       0xff0000,
			Description: fmt.Sprintf("You need to input currency!\nExample: BTC,ETH,DOGE,LTC,XMR,SHIBA,BNB"),
			Footer:      footer,
		})
		return
	}
	s.ChannelTyping(m.ChannelID)
	url := "https://api.nomics.com/v1/currencies/ticker?key=9bb9a6f4dfcd83c6541d871b90520475450365a8&ids=" + msglist[1] + "&interval=1h&convert=EUR&per-page=100&page=1"
	res, err := http.Get(url)
	if err != nil {
		s.ChannelMessageSendEmbed(m.ChannelID, &discordgo.MessageEmbed{
			Color:       0xff0000,
			Description: fmt.Sprintf("Sorry can't get price for inputed currency!"),
			Footer:      footer,
		})
		return
	}
	defer res.Body.Close()
	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		s.ChannelMessageSendEmbed(m.ChannelID, &discordgo.MessageEmbed{
			Color:       0xff0000,
			Description: fmt.Sprintf("Sorry can't get price for inputed currency!"),
			Footer:      footer,
		})
		return
	}
	var data []coinsData
	err = json.Unmarshal(body, &data)
	if err == nil {
		for _, values := range data {
			if values.Symbol == msglist[1] {
				max_supply := "Unknown"
				if values.MaxSupply != "" {
					max_supply = values.MaxSupply
				}
				s.ChannelMessageSendEmbed(m.ChannelID, &discordgo.MessageEmbed{
					Color: 0x00ff00,
					Title: "📊・Crypto Price Stats | 1h",
					Fields: []*discordgo.MessageEmbedField{
						{Name: "Price", Value: codeBlock(values.Price), Inline: true},
						{Name: "High price", Value: codeBlock(values.High), Inline: true},
						{Name: "Currency", Value: codeBlock(values.Symbol), Inline: true},
						{Name: "Status", Value: codeBlock(values.Status), Inline: true},
						{Name: "Name:", Value: codeBlock(values.Name), Inline: true},
						{Name: "Rank:", Value: codeBlock(values.Rank), Inline: true},
						{Name: "Circulating supply", Value: codeBlock(values.CirculatingSupply), Inline: true},
						{Name: "Max supply", Value: codeBlock(max_supply), Inline: true},
						{Name: "Market cap", Value: codeBlock(values.Marketcap), Inline: true},
						{Name: "First candle", Value: codeBlock(values.FirstCandle), Inline: true},
						{Name: "First trade", Value: codeBlock(values.FirstTrade), Inline: true},
					},
				})
				return
			}
		}
	}
	s.ChannelMessageSendEmbed(m.ChannelID, &discordgo.MessageEmbed{
		Color:       0xff0000,
		Description: fmt.Sprintf("Sorry, can't find that cryptocurrency!"),
		Footer:      footer,
	})
}
