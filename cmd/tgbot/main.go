package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/neuron-digital/go-prometheus-tgbot/jira"
	"github.com/neuron-digital/go-prometheus-tgbot/lang"
	"github.com/pkg/errors"
	"github.com/prometheus/common/model"
	"gopkg.in/alecthomas/kingpin.v2"
	"gopkg.in/telegram-bot-api.v4"
	"html/template"
	"io/ioutil"
	"log"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
	"strconv"
)

var (
	host                 = kingpin.Flag("host", "HTTP server host").Short('h').Default("0.0.0.0").String()
	port                 = kingpin.Flag("port", "HTTP server port").Short('p').Default("8080").Int()
	chat                 = kingpin.Flag("chat", "Telegram chat ID").Required().Int64()
	token                = kingpin.Flag("token", "Telegram TOKEN").Required().String()
	pollUpdates          = kingpin.Flag("poll", "Poll telegram updates").Default("false").Bool()
	alertManagerEndpoint = kingpin.Flag("alert-manager", "Alertmanager endpoint").String()
	templatesPath        = kingpin.Flag("templates-path", "Path to GO templates").Default("/opt/tgbot/templates").ExistingDir()
	tjMapArg             = kingpin.Flag("telegram-jira-map", "Format: <telegram_user_id>,<jira_user_name>;[...<telegram_user_id>,<jira_user_name>]").String()

	zeroDate     = time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC)
	muted        = zeroDate
	appStartTime = time.Now()
	usersTickets = make(map[int]CreateTicketRequest)
	tjMap        = make(map[int]string)
)

type AlertManagerRequest struct {
	Alerts model.Alerts `json:"alerts"`
}

type AlertsResponse struct {
	Status string       `json:"status"`
	As     model.Alerts `json:"data"`
}

func GetTranslation(msg tgbotapi.Message) lang.Translation {
	fmt.Println(msg.From.LanguageCode)
	if translation, ok := lang.Lang[msg.From.LanguageCode]; ok {
		return translation
	} else {
		return lang.Lang["en"]
	}
}

func GetKeyboard(update tgbotapi.Update) interface{} {
	// Send greeting and reply buttons
	var buttons []tgbotapi.KeyboardButton

	if _, ok := usersTickets[update.Message.From.ID]; ok {
		buttons = tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(GetTranslation(*update.Message).Cancel),
		)
	} else {
		buttons = tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton(GetTranslation(*update.Message).CreateTicket),
		)
	}

	return tgbotapi.NewReplyKeyboard(buttons)
}

func MessageTextIsCommand(update tgbotapi.Update) bool {
	translation := GetTranslation(*update.Message)
	if update.Message.Text == translation.CreateTicket || update.Message.Text == translation.Cancel {
		return true
	}

	return false
}

func execTelegramCommands(updates tgbotapi.UpdatesChannel, messages chan<- BotMessage) {
	for update := range updates {
		if update.Message == nil || update.Message.Time().Before(appStartTime) {
			continue
		}

		translation := GetTranslation(*update.Message)
		// Some command (/command or message text)
		command := ""

		if update.Message.IsCommand() {
			command = update.Message.Command()
		} else {
			if MessageTextIsCommand(update) {
				command = update.Message.Text
			}
		}

		if command != "" {
			switch command {
			case "mute":
				args := strings.TrimSpace(update.Message.CommandArguments())
				if args == "" {
					args = "5m"
				}

				duration, err := time.ParseDuration(args)
				if err == nil {
					messages <- BotMessage{
						Chat:      update.Message.Chat.ID,
						Text:      mute(duration),
						ParseMode: tgbotapi.ModeHTML,
						Mutable:   false,
					}

					go func(m time.Time) {
						time.Sleep(duration)
						if muted == m {
							messages <- BotMessage{
								Chat:      update.Message.Chat.ID,
								Text:      unmute(),
								ParseMode: tgbotapi.ModeHTML,
								Mutable:   false,
							}
						}
					}(muted)
				} else {
					messages <- BotMessage{
						Chat:      update.Message.Chat.ID,
						Text:      fmt.Sprintf("<b>%s</b>", err.Error()),
						ParseMode: tgbotapi.ModeHTML,
						Mutable:   false,
					}
				}

			case "unmute":
				messages <- BotMessage{
					Chat:      update.Message.Chat.ID,
					Text:      unmute(),
					ParseMode: tgbotapi.ModeHTML,
					Mutable:   false,
				}

			case "alerts":
				if alerts, err := composeAlertsMessages(); err == nil {
					for _, alert := range alerts {
						messages <- BotMessage{
							Chat:      update.Message.Chat.ID,
							Text:      alert,
							ParseMode: tgbotapi.ModeHTML,
							Mutable:   false,
						}
					}
				} else {
					messages <- BotMessage{
						Chat:      update.Message.Chat.ID,
						Text:      err.Error(),
						ParseMode: tgbotapi.ModeHTML,
						Mutable:   false,
					}
				}

			case "uid":
				messages <- BotMessage{
					Chat:      update.Message.Chat.ID,
					Text:      fmt.Sprintf("Your UserID is %d", update.Message.From.ID),
					ParseMode: tgbotapi.ModeHTML,
					Mutable:   false,
				}

			case "start":
				messages <- BotMessage{
					Mutable:     false,
					ParseMode:   tgbotapi.ModeMarkdown,
					Chat:        update.Message.Chat.ID,
					Text:        translation.Greeting,
					ReplyMarkup: GetKeyboard(update),
				}
			case translation.CreateTicket:
				// Button "Create ticket" pressed
				usersTickets[update.Message.From.ID] = CreateTicketRequest{Fields: Fields{
					Project:   Project{Key: "INFRA"},
					IssueType: IssueType{Id: 10001},
					Reporter:  &Person{Name: "admin"}, // Lookup reporter by From.ID
				}}
				messages <- BotMessage{
					Chat:        update.Message.Chat.ID,
					Text:        translation.EnterTicketTitle,
					ParseMode:   tgbotapi.ModeMarkdown,
					ReplyMarkup: GetKeyboard(update),
					Mutable:     false,
				}
			case translation.Cancel:
				// Button "Cancel" pressed
				delete(usersTickets, update.Message.From.ID)
				messages <- BotMessage{
					Chat:        update.Message.Chat.ID,
					Text:        translation.TicketCanceled,
					ParseMode:   tgbotapi.ModeMarkdown,
					ReplyMarkup: GetKeyboard(update),
					Mutable:     false,
				}
			}
		} else {
			if update.Message.Text != "" {
				if ticket, ok := usersTickets[update.Message.From.ID]; ok {
					switch "" {
					case ticket.Fields.Summary:
						ticket.Fields.Summary = update.Message.Text
						usersTickets[update.Message.From.ID] = ticket
						messages <- BotMessage{
							Chat:        update.Message.Chat.ID,
							Text:        translation.EnterTicketDescription,
							ParseMode:   tgbotapi.ModeMarkdown,
							ReplyMarkup: GetKeyboard(update),
							Mutable:     false,
						}
						fmt.Println(usersTickets[update.Message.From.ID].Fields.Summary)
					case ticket.Fields.Description:
						ticket.Fields.Description = update.Message.Text
						usersTickets[update.Message.From.ID] = ticket
						fmt.Println(usersTickets[update.Message.From.ID].Fields.Description)

						delete(usersTickets, update.Message.From.ID)
						if createdTicket, err := SendTicket(ticket); err != nil {
							messages <- BotMessage{
								Chat:        update.Message.Chat.ID,
								Text:        err.Error(),
								ParseMode:   tgbotapi.ModeMarkdown,
								ReplyMarkup: GetKeyboard(update),
								Mutable:     false,
							}
						} else {
							fmt.Println(createdTicket)
							messages <- BotMessage{
								Chat:        update.Message.Chat.ID,
								Text:        translation.TicketCreated,
								ParseMode:   tgbotapi.ModeMarkdown,
								ReplyMarkup: GetKeyboard(update),
								Mutable:     false,
							}

						}
					}
				}
			}
		}

	}
}

func SendTicket(ticket CreateTicketRequest) (interface{}, error) {
	var b bytes.Buffer
	jsonEncoder := json.NewEncoder(&b)
	errEncode := jsonEncoder.Encode(ticket)
	if errEncode != nil {
		return struct{}{}, errEncode
	}

	request, err := http.NewRequest("POST", "http://localhost:9005/rest/api/2/issue/", bufio.NewReader(&b))
	if err != nil {
		return struct{}{}, err
	}
	request.SetBasicAuth("admin", "admin")
	request.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	response, err := client.Do(request)
	if err != nil {
		return struct{}{}, err
	}

	var b2 []byte
	response.Body.Read(b2)
	response.Body.Close()

	if response.StatusCode == 201 {
		return struct{}{}, nil // TODO parse response
	} else {
		return struct{}{}, errors.New(response.Status)
	}

	// Create Jira ticket

}

type Project struct {
	Id  int    `json:"id,omitempty"`
	Key string `json:"key,omitempty"`
}

type IssueType struct {
	Id   int    `json:"id,omitempty"`
	Name string `json:"name,omitempty"`
}

type Person struct {
	Name string `json:"name,omitempty"`
}

type Fields struct {
	Project     Project   `json:"project,omitempty"`
	Summary     string    `json:"summary,omitempty"`
	IssueType   IssueType `json:"issuetype,omitempty"`
	Assignee    *Person   `json:"assignee,omitempty"`
	Reporter    *Person   `json:"reporter,omitempty"`
	Description string    `json:"description,omitempty"`
}

type CreateTicketRequest struct {
	Fields Fields `json:"fields"`
}

func mute(duration time.Duration) string {
	muted = time.Now().Add(duration)
	return fmt.Sprintf(`<b>Muted until %s</b>`, muted.Format("02.01.2006 15:04:05 MST"))
}

func unmute() string {
	if muted.IsZero() {
		return "<b>Oh, I am not muted!</b>"
	}

	muted = zeroDate
	return "<b>Unmuted</b>"
}

type TemplateName string

// composeAlertsMessages fetch GET alerts from alertsmanager and render them by template.
// Alerts are grouped by alert label "job".
func composeAlertsMessages() (messages []string, err error) {
	defer func() {
		if r := recover(); r != nil {
			messages, err = nil, r.(error)
		}
	}()

	endpoint := strings.TrimRight(*alertManagerEndpoint, "/")
	alertsLocation := fmt.Sprintf("%s/api/v1/alerts", endpoint)

	resp, err := http.Get(alertsLocation)
	if err != nil {
		panic(err)
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		panic(err)
	}

	var alertsResponse AlertsResponse

	if err := json.Unmarshal(body, &alertsResponse); err != nil {
		panic(err)
	}

	// Group by job: map[job][]alert
	alerts := make(map[string][]string)

	for _, alert := range alertsResponse.As {
		alert.Status()
		text, err := renderAlert(alert, TemplateName("alert.html"))
		if err != nil {
			text = err.Error()
		}

		job := string(alert.Labels[model.LabelName("job")])
		if _, ok := alerts[job]; ok {
			alerts[job] = append(alerts[job], text)
		} else {
			alerts[job] = []string{text}
		}
	}

	// Get jobs to sort by them
	var jobs []string
	for job, _ := range alerts {
		jobs = append(jobs, job)
	}

	sort.Strings(jobs)

	// Messages sorted by job
	for _, job := range jobs {
		a := alerts[job]
		messages = append(messages, fmt.Sprintf("\n<b>%s:</b>", strings.Replace(strings.ToUpper(job), "_", " ", len(job))))
		for _, text := range a {
			messages = append(messages, text)
		}
	}

	return []string{strings.Join(messages, "\n")}, nil
}

// renderAlert renders alert template
func renderAlert(a *model.Alert, tpl TemplateName) (string, error) {
	var text bytes.Buffer
	t, err := template.New("").Funcs(template.FuncMap{
		"label": func(s model.LabelSet, l string) string { return string(s[model.LabelName(l)]) }}).ParseFiles(fmt.Sprintf("%s/%s", *templatesPath, tpl))
	if err != nil {
		return "", err
	}

	if execute_err := t.ExecuteTemplate(&text, string(tpl), a); execute_err != nil {
		return "", execute_err
	}

	return text.String(), nil
}

// sendTelegramMessages send all channel messages to telegram channel
func sendTelegramMessages(bot *tgbotapi.BotAPI, messages <-chan BotMessage) {
	for msg := range messages {
		if msg.Mutable && time.Now().Before(muted) {
			continue
		}

		m := tgbotapi.NewMessage(msg.Chat, msg.Text)
		m.ParseMode = msg.ParseMode
		m.DisableWebPagePreview = true
		m.ReplyMarkup = msg.ReplyMarkup
		bot.Send(m)
	}
}

func initBot() *tgbotapi.BotAPI {
	bot, err := tgbotapi.NewBotAPI(*token)
	if err != nil {
		log.Panic(err)
	}

	log.Printf("Telegram bot authorized: %s", bot.Self.UserName)
	return bot
}

func initTelegramUpdatesChannel(bot *tgbotapi.BotAPI) tgbotapi.UpdatesChannel {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates, _ := bot.GetUpdatesChan(u)
	return updates
}

type View struct {
	channel chan BotMessage
}

func (view *View) handleJira(w http.ResponseWriter, r *http.Request) {
	event := jira.Event{}
	body, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.Print(err)
	} else {
		json.Unmarshal(body, &event)
		view.channel <- BotMessage{
			Chat:      *chat,
			Text:      event.ComposeMessage(),
			ParseMode: tgbotapi.ModeHTML,
			Mutable:   true,
		}
	}

}

func (view *View) handleAlerts(w http.ResponseWriter, r *http.Request) {
	templateName := strings.TrimSpace(r.URL.Query().Get("template"))
	if templateName == "" {
		templateName = "alert.html"
	}

	processAlert := func(alert *model.Alert, wg *sync.WaitGroup) {
		text, err := renderAlert(alert, TemplateName(templateName))
		if err == nil {
			view.channel <- BotMessage{
				Chat:      *chat,
				Text:      text,
				ParseMode: tgbotapi.ModeHTML,
				Mutable:   true,
			}
		} else {
			log.Print(err)
		}
		wg.Done()
	}

	var amRequest AlertManagerRequest
	body, err := ioutil.ReadAll(r.Body)
	log.Println("Alertmanager request body:")
	log.Print(string(body))

	if err != nil {
		log.Print(err)
	} else {
		err := json.Unmarshal(body, &amRequest)
		if err != nil {
			log.Print(err)
		} else {
			log.Printf("Processing %d alerts...\n", len(amRequest.Alerts))
			wg := sync.WaitGroup{}
			wg.Add(len(amRequest.Alerts))
			for _, alert := range amRequest.Alerts {
				go processAlert(alert, &wg)
			}
			wg.Wait()
			log.Println("Processing alerts done.")
		}
	}
}

func (view *View) sendMessage(w http.ResponseWriter, r *http.Request) {
	message := r.FormValue("message")
	if len(message) > 0 {
		view.channel <- BotMessage{
			Chat:      *chat,
			Text:      message,
			ParseMode: tgbotapi.ModeHTML,
			Mutable:   true,
		}
	}
}

type BotMessage struct {
	Chat        int64
	Text        string
	ParseMode   string
	Mutable     bool
	ReplyMarkup interface{}
}

func parseTjMap() {
	if *tjMapArg != "" {
		rows := strings.Split(*tjMapArg, ";")
		for _, kv := range rows {
			cell := strings.Split(kv, ",")
			id, _ := strconv.ParseInt(cell[0], 10, 64)
			tjMap[int(id)] = cell[1]
		}
	}
}

func main() {
	kingpin.Parse()

	// Parse mapping of telegram and jira users
	parseTjMap()

	// Telegram bot
	bot := initBot()

	// Messages to send to telegram channel
	messages := make(chan BotMessage, 10)
	go sendTelegramMessages(bot, messages)

	log.Printf("Poll updates: %t", *pollUpdates)
	if *pollUpdates {
		go execTelegramCommands(initTelegramUpdatesChannel(bot), messages)
	}

	view := View{channel: messages}
	http.HandleFunc("/api/v1/alert", view.handleAlerts)
	http.HandleFunc("/api/v1/message", view.sendMessage)
	http.HandleFunc("/api/v1/jira", view.handleJira)

	log.Printf(`Listen %s:%d`, *host, *port)
	http.ListenAndServe(fmt.Sprintf("%s:%d", *host, *port), nil)

	// TODO accept commands only from users in tjMap
}
