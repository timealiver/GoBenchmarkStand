# GoStand

Экспериментальный стенд на **Go**: один и тот же сценарий обработки HTTP-запроса реализован **четырьмя способами**, чтобы сравнить влияние приёмов оптимизации на память, аллокации и задержку. Проект задуман как основа для доклада, статей и воспроизводимых замеров — не как production-сервис.

## Что здесь происходит

- Клиент шлёт **POST** с JSON-массивом «событий» (метрики).
- Сервер **фильтрует** записи по порогу `value > 50`, считает **count, sum, mean, p50, p95, p99** и отвечает JSON.
- Четыре эндпоинта отличаются только реализацией парсинга/буферов/структур данных (см. таблицу ниже).

| Эндпоинт | Идея |
|----------|------|
| `POST /v1/aggregate` | «Как в туториале»: `encoding/json`, декодер с тела запроса, без пулов. |
| `POST /v2/aggregate` | Пулы `sync.Pool` для буфера и слайсов, `Unmarshal`/`Marshal` с переиспользованием. |
| `POST /v3/aggregate` | Плоские структуры без указателей в горячих данных, интернирование имён, `unsafe` для копирования id; меньше работы для GC при сканировании. |
| `POST /v4/aggregate` | **Sonic** (`github.com/bytedance/sonic`) + плоские структуры — быстрый JSON без стандартного `encoding/json` на горячем пути. |

Дополнительно в репозитории есть скрипты для **нагрузки** (vegeta), **микробенчмарков** (`go test -bench`) и **HTML-отчёта** с графиками.

## Быстрый старт

Нужен Go **1.21+** (в `go.mod` указана используемая версия).

```bash
git clone <repo-url>
cd <каталог-репозитория>
```

### 1. Сгенерировать JSON-пейлоады для тестов

Без этих файлов бенчи пропустятся или упадут.

```bash
go run ./bench/gen -out bench/payloads
```

Появятся `payload_100.json`, `payload_1k.json`, `payload_10k.json`, `payload_50k.json`.

### 2. Запустить сервер

```bash
go run ./cmd/server -addr :8080 -out results -version all
```

Эндпоинты: `/v1/aggregate` … `/v4/aggregate`. Метрики в фоне пишутся в `results/metrics_*.jsonl` (если нужно).

Полезно для отладки:

- `GET /debug/metrics` — снимок `runtime.MemStats` в JSON.
- `GET /debug/pprof/` — стандартный pprof (профиль для PGO снимать отсюда же).

### 3. Нагрузочный тест (vegeta)

Нужен установленный [vegeta](https://github.com/tsenart/vegeta). На Linux/macOS:

```bash
bash scripts/load_test.sh
```

На Windows смотрите параметры в `scripts/load_test.sh` и адаптируйте под PowerShell или WSL.

### 4. Микробенчмарки

Из корня репозитория (UTF-8 вывод важен для `benchstat` и генератора отчёта на Windows — см. ниже).

**Один прогон (PowerShell):**

```powershell
go run ./bench/gen -out bench/payloads
go test ./bench/ -bench=BenchmarkHandlers -benchmem -count=10 -timeout=900s 2>&1 |
  Out-File -FilePath results/bench_run.txt -Encoding utf8
```

**Статистика двух файлов в консоли** (`benchstat`):

```bash
go install golang.org/x/perf/cmd/benchstat@latest
benchstat results/run1.txt results/run2.txt
```

#### Два прогона: Windows на хосте и Linux в Docker

Типичный сценарий — сравнить **тот же код** на Windows и в Linux-контейнере (например, чтобы увидеть разницу по Sonic). Нужны [Docker](https://docs.docker.com/get-docker/) и этот репозиторий на диске.

1. **Windows (локально):** пейлоады и бенч, результат в UTF-8:

```powershell
go run ./bench/gen -out bench/payloads
go test ./bench/ -bench=BenchmarkHandlers -benchmem -count=10 -timeout=900s 2>&1 |
  Out-File -FilePath results/bench_windows.txt -Encoding utf8
```

(В Windows PowerShell 5.1 у `Tee-Object` нет параметра `-Encoding`; для вывода в UTF-8 используйте `Out-File -Encoding utf8` или PowerShell 7+.)

2. **Linux внутри Docker** — образ [`Dockerfile`](Dockerfile) основан на **`golang:bookworm`** и `GOTOOLCHAIN=auto`, чтобы при необходимости подтянуть версию Go из `go.mod` (например 1.25.x), даже если в базовом образе Go чуть старше. Пейлоады создаются при сборке образа.

```powershell
.\scripts\bench_docker.ps1
```

Или в Git Bash / WSL:

```bash
bash scripts/bench_docker.sh
```

По умолчанию вывод сохраняется в `results/bench_linux.txt` (путь можно передать первым аргументом скрипта).

3. **HTML-отчёт** с таблицей A / B / Δ и графиками по **прогону A** (`-in`):

```bash
go run ./bench/report/ -in results/bench_windows.txt -cmp results/bench_linux.txt -out results/report_win_vs_linux.html
```

Если нужны графики именно по Linux, поменяйте местами: `-in results/bench_linux.txt -cmp results/bench_windows.txt`.

Имя образа можно задать переменной `GOSTAND_BENCH_IMAGE`.

### 5. HTML-отчёт с графиками

Собирает интерактивные графики (Chart.js с CDN) и **фиксирует среду** на момент генерации: версия Go, `GOOS`/`GOARCH`, `GOMAXPROCS`, версия зависимости Sonic, имя хоста.

Только один прогон (графики и «прогон A» в таблице):

```bash
go run ./bench/report/ -in results/bench_run.txt -out results/report.html
```

**Два прогона** — таблица с колонками A / B / Δ и отдельный график Δ по времени для размера **1k**:

```bash
go run ./bench/report/ -in results/before.txt -cmp results/after.txt -out results/report.html
```

Смысл Δ: \((B - A) / A \cdot 100\%\). Для времени и аллокаций **отрицательный** процент означает улучшение во втором прогоне.

> На Windows перенаправление вывода в файл иногда пишет UTF-16 — тогда `benchstat` и парсер отчёта видят «пустой» файл. Используйте `Out-File -Encoding utf8` в PowerShell или `tee` с UTF-8 в bash.

## Sonic на Windows — о чём оговорка

Библиотека **Sonic** ориентирована на максимальную скорость в типичной серверной среде **Linux amd64** (и частично другие Unix): там задействуется нативное / SIMD-ускоренное ядро парсера там, где это поддерживается сборкой.

На **Windows** чаще используется **запасной (совместимый) путь** без того же низкоуровневого ядра, что на Linux. В результате:

- V4 по-прежнему может быть **существенно быстрее**, чем «голый» `encoding/json`, но **цифры не переносятся один в один** с Linux.
- Для честного сравнения «как в продакшене» имеет смысл прогнать те же бенчмарки в **Linux** (железо, VM, WSL2, Docker) и указать ОС в отчёте — блок «Среда» в HTML это упрощает.

## PGO (Profile-Guided Optimization)

PGO в Go — **сборка всего бинарника** с профилем CPU, а не отдельной функции. Типичный сценарий:

1. Собрать бинарник без PGO, запустить под нагрузкой.
2. Снять профиль: `curl -o cpu.prof "http://localhost:8080/debug/pprof/profile?seconds=60"` (пока идёт нагрузка).
3. Положить профиль как `cmd/server/default.pgo` (или указать `-pgo=...` при сборке).
4. Пересобрать и сравнить задержки/бенчи с предыдущей сборкой.

Сравнение двух **прогонов бенчмарка** удобно оформлять через `benchstat` или через `go run ./bench/report/ -in ... -cmp ...`.

Подробности и параметры компилятора — в документации Go: [Profile-guided optimization](https://go.dev/doc/pgo).

## Структура репозитория

```
Dockerfile           образ для прогона бенчмарков под Linux
cmd/server/          точка входа HTTP-сервера
internal/handler/    v1–v4 обработчики
internal/model/       типы событий и ответа
internal/aggregate/  фильтрация и перцентили
internal/collector/  фоновый снимок MemStats (JSONL)
bench/               бенчмарки и генератор пейлоадов
bench/report/        генератор HTML-отчёта (шаблон report.html.tmpl)
scripts/             vegeta, Docker-бенч, анализ
pgo/                 каталог для cpu-профилей (артефакты не коммитятся)
results/             локальные результаты замеров (в .gitignore)
```
