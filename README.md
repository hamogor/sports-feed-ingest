# ECB Feed Sync Service

This service ingests content from the ECB content feed, stores articles in MongoDB and publishes update
notifications into RabbitMQ when an article is added or changed. An event component consumes MongoDB change streams
and publishes updates to a message bus.

## High Level Overview

`ECB Feed  →  Ingest Service  →  MongoDB  →  Change Stream  →  RabbitMQ`
1. Poll the external feed every few seconds
2. Detect new or changed articles
3. Upsert record into Mongo
4. Event service publishes message to RabbitMQ
5. Downstream consumers receive content updates

## Running Locally
You can run the service via

```docker-compose up```

This will start the service to ingest data aswell as an instance of MongoDB and RabbitMQ

To access containers

- MongoDB: `docker exec -it news-mongo mongosh`
- RabbitMQ: `http://localhost:15672`(login: gues/guest) 
- API Health: `curl localhost:8080/healthz`

### Configuration
Currently the configuration variables are defined within docker-compose.yml, they are:

| Value              | Usage                                           | Default                                                    |
|--------------------|-------------------------------------------------|------------------------------------------------------------|
| MONGO_URI          | Local instance of MongoDB                       | `"mongodb://localhost:27017"`                              |
| MONGO_DB_NAME      | Database name where articles are stored         | `"newsdb"`                                                 |
| FEED_URL           | URL of the ECB content feed                     | `"https://content-ecb.pulselive.com/content/ecb/text/EN/"` |
| PAGE_SIZE          | How many articles to fetch per page             | 20                                                         |
| MAX_POLLS          | How many times to poll the feed before stopping | -1                                                         |
| MAX_PAGES          | Maximum amount of pages to fetch                | -1                                                         |
| TIMEOUT            | HTTP Timeout duration to fetch ECB Feed         | `10 * time.Second`                                         |
| POLL_INTERVAL      | How long before polling again                   | `5 * time.Second`                                          |
| RABBIT_URI         | Local instance of RabbitMQ                      | `"amqp://guest:guest@localhost:5672/"`                     |
| RABBIT_EXCHANGE    | Exchange for local RabbitMQ                     | `"cms.sync"`                                               |
| RABBIT_ROUTING_KEY | Routing key for RabbitMQ                        | `"article.updated"`                                        |

## Testing
To run tests run (no docker required)
```go test ./...```

## Design and Behaviour

### Ingestion and Persistence

We page through the ECB feed on a schedule, page results safely, and upsert into MongoDB.
Failure is localised to a single run, trivial to control behaviour via configuration flags. 
In future I'd add a backoff on repeated failure. metrics for latency and error rates and alerting if polls
continuously failed.

There are a number of stop conditions on the poller

1. Three consecutive empty pages:

If we get 3 pages in a row with no content, we assume we've reached the end of useful data and log out

2. Advertised last page

If the API tells us there are `N` pages via `res.PageInfo.NumPages` we stop when we reach `page >= N` and log

3. Configured `MAX_PAGES` limit

Support is available for a hard cut-off:
- `MAX_PAGES = -1` -> ingest until feed says stop / empty pages
- `MAX_PAGES = N` -> never fetch more than N pages per run

This can allow partial ingest to smoke test

4. `AbsoluteMaxPages`

There is a compile time const that will prevent us from ever ingesting more than 5000 pages, this would obviously be subject
to more knowledge about the API to make a more reasonable assumption to what this value should be. 

#### Idempotency and deduplication
If the same article appears in multiple pages or multiple polls or the feed overlaps pages, to avoid writing the same article each `RunOnce` call
keeps a map of `seen` articles and will skip them 
```go
seen := make(map[int64]struct{})
...
if _, ok := seen[ecbArt.ID]; ok {
    continue // skip duplicates within this run
}
seen[ecbArt.ID] = struct{}{}
```
This allows each `externalId` to represent exactly one logical article.

#### Update Rules
The ingestion is designed so that a new `externalId` creates a document, re-ingesting an article only updates when the feed data is newer
based on `lastModified` either in the main article of its `leadMedia`

#### Batch Upsert
Per page we map ECB articles into `article.Article` and build a batch
- `BulkUpsert(ctx, []*article.Article` writes in one go
- we log how many documents actually changed for obs

This allows for fewer round trips to MongoDB and a clean seperation between ingestion and persistence

#### Events
When an article changes we publish an `article.updated` event and let other services subscribe.

- Decoupling: the ingest only knows how to publish an `article.updated` event. It doesn't care who's listening, new services can be added
without changes.
- Durability: RabbitMQ can persist messages to disk, which means if a consumer is down it won't miss updates. 
- Future proofing: By using an exchange + routing key we can route the same event to multiple queues or later introduce more routing keys if the 
domain grows.

In future I would lean toward a lighter payload, only including the fields that changed + IDs

## Other considerations

### MongoDB
- Look at a 3 node replica set with a primary and 2 secondaries for higher availability
- Backups & point in time recovery

### RabbitMQ
- Dead letter queues for repeatedly failing messages
- Retries with backoff on failures
- Monitoring & Metrics

### Ingestion
- Maybe run the poller as a dedicated service (overkill?)
- Add backoff and rate limits
- Monitoring & Metrics
- Graceful shutdown
- Current config is very light, could look at running batch after a couple of pages for less round trips.

### Other
- Don't store configuration in `docker-compose.yml`