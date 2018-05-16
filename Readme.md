## About
This application works as bridge between Alertmanager => Telegram and Jira => Telegram.

It starts Alertmanager webhook, Jira webhook and listens commands from telegram chat.

## Webhooks API
* `POST /api/v1/alerts` - Alertmanager webhook. More about request format is in Alertmanager documentaion.
* `POST /api/v1/jira` - Jira webhook. More about request format is in Jira documentation.

## Commands
* `/mute` - stop sending messages to chat.
* `/unmute` - continue sending messages to chat if was muted.
* `/alerts` - list current alerts from Alertmanager.

Commands will be unavailable if you run bot on channel (by nature of channels).


## Configuration

| CLI option     | Type    | Default              | Description                 |
|:---------------|:--------|:---------------------|:----------------------------|
| host           | string  | 0.0.0.0              | HTTP server bind host       |
| port           | int     | 8080                 | HTTP server bind port       |
| chat           | int     |                      | Telegram chat/channel ID    |
| token          | string  |                      | Telegram bot TOKEN          |
| poll           | boolean | false                | Do telegram updates polling |
| alert-manager  | string  |                      | Alertmanager endpoint       |
| templates-path | string  | /opt/tgbot/templates | Path to Go templates        |

## Docker
Docker container can be build as shown in [Makefile](Makefile-example)