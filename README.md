# TripGo Trip Service

HTTP-сервис на Go для создания поездок, поиска поездки по ID и её завершения. Сервис хранит поездки и историю их статусов в PostgreSQL.

## Требования

- Go 1.25.4 или совместимая версия.
- PostgreSQL. Для окружения курса используйте `tripgoctl` и файл `environment.toml` из этого репозитория.
- `goose` для управления миграциями.
- `golangci-lint` для запуска линтера.

## Запуск локального окружения

Установите и настройте `tripgoctl` согласно [инструкции курса](https://github.com/course-go-autumn-2026/course/blob/main/homework/docs/getting-started.md), затем из корня репозитория выполните:

```bash
tripgoctl cluster start
tripgoctl environment start
tripgoctl connect
```

`tripgoctl environment start` создаёт локальный `.env` с адресом PostgreSQL и остальными параметрами. `.env` не коммитится. Если вы запускаете сервис со своей базой, скопируйте `.env.example` в `.env` и укажите рабочий `DATABASE_URL`.

## Переменные окружения

Переменные в таблице ниже обязательны. Сервис завершает запуск с ошибкой, если переменная отсутствует или имеет неверный формат. Настройки OpenTelemetry перечислены отдельно.

| Переменная | Назначение | Пример |
|---|---|---|
| `HTTP_ADDR` | Адрес HTTP-сервера | `:8080` |
| `HTTP_READ_TIMEOUT` | Максимальное время чтения всего HTTP-запроса | `5s` |
| `HTTP_READ_HEADER_TIMEOUT` | Максимальное время чтения заголовков | `2s` |
| `HTTP_WRITE_TIMEOUT` | Максимальное время отправки ответа | `10s` |
| `HTTP_IDLE_TIMEOUT` | Максимальное время ожидания следующего запроса keep-alive | `60s` |
| `LOG_LEVEL` | Уровень логирования | `info` |
| `SHUTDOWN_TIMEOUT` | Лимит на завершение HTTP-сервера | `10s` |
| `DATABASE_URL` | Строка подключения PostgreSQL | `postgres://user:pass@localhost:5432/tripgo?sslmode=disable` |
| `DATABASE_MAX_CONNS` | Максимум соединений в пуле | `10` |
| `DATABASE_MIN_CONNS` | Минимум соединений в пуле | `2` |
| `DATABASE_MAX_CONN_LIFETIME` | Максимальное время жизни соединения | `30m` |
| `DATABASE_CONNECT_TIMEOUT` | Таймаут проверки подключения при старте | `5s` |
| `DATABASE_QUERY_TIMEOUT` | Таймаут запросов к PostgreSQL | `3s` |
| `IDEMPOTENCY_KEY_RETENTION` | Срок хранения ключей идемпотентности | `24h` |

Актуальный шаблон находится в [.env.example](.env.example). Таймауты и максимальное число соединений должны быть положительными; `DATABASE_MIN_CONNS` допускает ноль и не должно превышать `DATABASE_MAX_CONNS`.

Настройки телеметрии для запуска на хосте с окружением `tripgo-lab-02`:

| Переменная | Значение |
|---|---|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | `http://localhost:22318` — сверить с `tripgoctl connect` |
| `OTEL_EXPORTER_OTLP_PROTOCOL` | `http/protobuf` |
| `OTEL_TRACES_SAMPLER` | `parentbased_always_on` |
| `OTEL_METRIC_EXPORT_INTERVAL` | `1000` мс для локальной проверки |
| `OTEL_BSP_SCHEDULE_DELAY` | `1000` мс для локальной проверки |

SDK допускает значения по умолчанию, но endpoint необходимо настроить на коллектор своего окружения.

## Миграции и запуск

Из корня проекта:

```bash
make migrate
make run
```

Сервис слушает адрес из `HTTP_ADDR`. Для миграций доступны также `make migrate-status` и `make migrate-down`. Все изменения схемы выполняются Goose-миграциями из `migrations/`.

Чтобы остановить сервис, отправьте `SIGINT` или `SIGTERM` (например, нажмите `Ctrl+C`). HTTP-сервер прекращает принимать новые запросы и ждёт текущие в пределах `SHUTDOWN_TIMEOUT`; если время истекло, активные соединения закрываются принудительно, после чего закрывается пул PostgreSQL.

## Docker

Сборка использует многоступенчатый Dockerfile: приложение компилируется в Go builder-образе, а итоговый образ содержит только статический бинарник и сертификаты CA. Контейнер запускается от непривилегированного пользователя.

```bash
docker build -f deploy/Dockerfile -t tripgo-trip-service .

docker compose -f deploy/compose.yaml up -d --build
docker compose -f deploy/compose.yaml logs -f
```

Для окружения `tripgo-lab-02` Compose переопределяет адреса из `.env`:
PostgreSQL — `host.docker.internal:22032`, OTLP HTTP —
`http://host.docker.internal:22318`. В Docker Desktop это адрес хоста;
`localhost` внутри контейнера указывал бы на сам контейнер сервиса.
При запуске через `make run` используются адреса `localhost` из `.env`.
Если утилита изменит порты, обновите их в `deploy/compose.yaml` по выводу
`tripgoctl connect`.

## Реализовано

| Метод и путь | Поведение |
|---|---|
| `POST /api/v1/trips` | Создаёт активную поездку и запись истории статуса. Возвращает `201` и `Location`. Если у водителя уже есть активная поездка, возвращает `409` с кодом `driver_busy`. При повторе с тем же `Idempotency-Key` и телом в течение `IDEMPOTENCY_KEY_RETENTION` возвращает сохранённый ответ с `200`; тот же ключ с другим телом даёт `409 idempotency_conflict`. По истечении срока ключ можно использовать повторно. |
| `GET /api/v1/trips/{tripId}` | Возвращает поездку; отвечает `400` для некорректного UUID и `404`, если поездка не найдена. |
| `POST /api/v1/trips/{tripId}/finish` | Завершает активную поездку и записывает переход статуса в той же транзакции. Повторное завершение возвращает `409`, неизвестный ID — `404`. |
| `GET /health` | Проверка liveness процесса. |
| `GET /ready` | Проверяет подключение к PostgreSQL; возвращает `503`, если база недоступна. |

Описание HTTP-контракта находится в [contracts/openapi/trip-service.openapi.yaml](contracts/openapi/trip-service.openapi.yaml). Контракты курса и схему таблиц не редактируйте; таблицы создаются миграциями.

### Основные решения

- Поездка и запись истории её статуса создаются в одной транзакции.
- Пул PostgreSQL создаётся с ограничениями `DATABASE_MAX_CONNS`, `DATABASE_MIN_CONNS` и `DATABASE_MAX_CONN_LIFETIME`; настройки HTTP-таймаутов берутся из `.env`.
- Частичный уникальный индекс `trips_one_active_trip_per_driver_idx` не допускает две активные поездки одного водителя, в том числе при параллельных запросах. Нарушение индекса при `INSERT` переводится в `ErrDriverBusy` и HTTP `409`; отдельной предварительной проверки нет.
- Middleware идемпотентности резервирует ключ, выполняет создание поездки и сохраняет ответ в одной транзакции. Для сравнения запросов хранятся SHA-256 и каноническое тело; совпадения одного хеша недостаточно.
- Завершение — атомарный `UPDATE`, который изменяет только строку со статусом `active`. Обновление поездки и запись истории выполняются в одной транзакции.
- HTTP-ошибки возвращаются в формате `application/problem+json` с машиночитаемым `code`.

## Проверки

```bash
make test
make lint
```

`make test` запускает тесты с `-race` и печатает покрытие. `make lint` запускает `golangci-lint`. Также можно проверить компиляцию командой `go build ./...`.

Unit-тесты и интеграционные сценарии описаны ниже.

## Структура проекта

- `cmd/trip-service/` — запуск и остановка приложения; сборка зависимостей находится в `internal/container/`.
- `internal/` — слои приложения, инфраструктура и конфигурация.
- `internal/generated/` — код, сгенерированный из OpenAPI; вручную его не редактируйте.
- `contracts/` — API-контракты и описание схемы.
- `migrations/` — миграции Goose.

## Архитектура

`main → container → HTTP → usecase → domain`.
`container` также создаёт репозитории и инфраструктуру PostgreSQL/OTel;
репозитории реализуют интерфейсы потребителей из usecase.
Каждое действие (`create`, `get`, `finish`) имеет отдельный пакет на слоях
HTTP и usecase. Неэкспортируемые интерфейсы расположены в `contract.go`
потребителя: ему нужен только используемый набор операций. Моки генерируются
через `go:generate` и хранятся рядом в `mocks_test.go`.

Домен содержит модель, валидацию и переход `Finish`, не зависит от HTTP,
SQL или OpenTelemetry. Отдельный `model` содержит запись идемпотентности;
модель поездки находится рядом с правилами в `domain/trip`, чтобы не создавать
циклическую зависимость. Транспорт преобразует входной OpenAPI DTO в доменную
модель, ошибки переводит в Problem пакет `http/domainerror`. Контракт OpenAPI
и миграции сохранены.

Создание и завершение записывают поездку и историю в одной транзакции.
Менеджер транзакций переиспользует транзакцию из контекста: это сохраняет
атомарность идемпотентного создания. Завершение сначала блокирует строку
`FOR UPDATE`, затем вызывает доменный переход. Все реализации связываются
в `container.New`; `main` читает env и управляет жизненным циклом.
Второй транспорт сможет вызывать те же usecase без зависимости от HTTP.

## Проверки

- `make generate` — обновить OpenAPI и gomock-моки.
- `make lint` — архитектурные ограничения depguard и остальные линтеры.
- `make test` — unit/httptest с race detector, покрытие в `coverage.out`
  и итоговая таблица покрытия.
- `make test-integration` — PostgreSQL-тесты под тегом `integration`.
  Сначала `tripgoctl environment start`, экспортировать `DATABASE_URL`,
  применить существующие миграции через `make migrate`.
  Тесты используют случайные UUID и удаляют свои строки; проверка rollback
  вставляет поездку и историю, затем нарушает CHECK истории и проверяет обе таблицы.


## Наблюдаемость

Логгер — `slog`, JSON в stdout, уровень `LOG_LEVEL` (`INFO`, `DEBUG`, `WARN`,
`ERROR`). Handler добавляет `trace_id` и `span_id` из контекста каждой записи.
Бизнес-события пишутся после фактического коммита транзакции, включая внешнюю транзакцию идемпотентности.

Экспорт OTLP настраивается:

```sh
export OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:22318
export OTEL_EXPORTER_OTLP_PROTOCOL=http/protobuf
export OTEL_TRACES_SAMPLER=parentbased_always_on
```

HTTP: Recover → Tracing → Logging → Metrics → router. Маршрут предварительно
сопоставляется с шаблоном без выполнения обработчика. В метрики и имя спана
попадает шаблон, никогда UUID или query string. Recover расположен снаружи Tracing. Logging и Metrics завершаются через defer
при панике, сохраняя статус 500. Ответ буферизуется, чтобы Recover мог заменить
даже частично подготовленный JSON на единый internal_error. Контекст спана
сохраняется для записи паники со stack trace.

| Сигнал | Тип, единица | Атрибуты |
| --- | --- | --- |
| `http.server.request.duration` | histogram, s | `http.request.method`, `http.route`, `http.response.status_code` |
| `http.server.active_requests` | up-down counter | `http.request.method`, `http.route` |
| `db.client.operation.duration` | histogram, s | `db.operation.name`, `db.collection.name`, `error.type` |
| `tripgo.trips.created` | counter | — |
| `tripgo.trips.completed` | counter | — |

`otelpgx` подключён один раз к пулу. Инфраструктурный tracer добавляет
стабильные атрибуты операции/таблицы и длительность в секундах. SQL-параметры
и тела HTTP не экспортируются. На серверных ошибках HTTP спан получает Error
и `error.type=internal_error`. База использует `database_error`.


