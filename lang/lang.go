package lang

type Translation struct {
	EnterTicketTitle       string
	EnterTicketDescription string
	CreateTicket           string
	Cancel                 string
	TicketCreated          string
	TicketCanceled         string
	Greeting               string
}

var ru = Translation{
	EnterTicketTitle:       "Введите название тикета",
	EnterTicketDescription: "Введите описание тикета",
	TicketCreated:          "Тикет создан!",
	TicketCanceled:         "Тикет отменён",
	Greeting:               `Вас приветствует телеграм бот Jira!`,
	CreateTicket:           "🖋 Создать тикет",
	Cancel:                 "🚫 Отмена",
}

var en = Translation{
	EnterTicketTitle:       "Enter ticket title",
	EnterTicketDescription: "Enter ticket description",
	TicketCreated:          "Ticket created!",
	TicketCanceled:         "Ticket canceled",
	Greeting:               `Telegram bot for Jira welcome you!`,
	CreateTicket:           "🖋 Create ticket",
	Cancel:                 "🚫 Cancel",
}

var Lang = map[string]Translation{"ru": ru, "en": en, "en-US": en, "ru-RU": en}
