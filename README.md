# Pivotal Tracker Slack Bot

Slack bot that extracts pivotal tracker story IDs from the channel messages and
automatically provides descriptions for them.

## Configuration

Environment variables:

* `PORT`
* `SLACK_TOKEN`
* `SLACK_SIGNING_SECRET`
* `PIVOTAL_TOKEN`

## Deployment

### Docker Compose

Edit `./configs/.env.prod` file to add all required configuration parameters.

Then run:

```console
$ docker-compose -f ./deployment/docker-compose.yml --env-file ./configs/.env.prod up -d
```

To update:

```console
$ docker-compose -f ./deployment/docker-compose.yml pull
$ docker-compose -f ./deployment/docker-compose.yml --env-file ./configs/.env.prod up --no-deps -d web
```
