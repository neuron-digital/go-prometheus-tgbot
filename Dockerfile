FROM alpine

RUN apk add --no-cache ca-certificates
ADD tgbot.build /usr/bin/tgbot
ADD templates /opt/tgbot/templates

VOLUME ["/opt/tgbot/templates"]

ENTRYPOINT ["/usr/bin/tgbot"]
