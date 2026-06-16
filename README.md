# DoT n DoH Checker

Утилита на Go для поиска самых быстрых публичных DNS-over-TLS и
DNS-over-HTTPS резолверов из вашей текущей сети.

Идея простая: вместо обычного ICMP ping программа делает настоящие DNS-запросы
через DoT и DoH, замеряет задержку каждого ответа, сортирует результаты и
показывает лучшие endpoint'ы для настройки в роутере, AdGuard Home, dnsmasq,
sing-box, Clash, Keenetic, OpenWrt и похожих системах.

## Возможности

- Проверяет DoT через TLS на порту `853`.
- Проверяет DoH через `application/dns-message`.
- Читает кандидатов из простых файлов `DoT.txt` и `DoH.txt`.
- Показывает `median`, `average`, `min`, `max` и количество успешных запросов.
- Сортирует сначала по успешности, затем по медианной задержке.
- По умолчанию выводит топ-4 DoT и топ-4 DoH.
- В комплекте уже есть по 50 бесплатных публичных DoT и DoH endpoint'ов.

## Быстрый старт

Нужен Go `1.22+`.

```powershell
go run .
```

Пример:

```powershell
go run . -count 10 -top 4 -domain google.com
```

Только DoT:

```powershell
go run . -only dot
```

Только DoH:

```powershell
go run . -only doh
```

## Сборка и архив

Если установлен `make`, соберите готовый пакет одной командой:

```powershell
make build
```

Команда создаст:

```text
dist/
  dotndoh-checker.exe
  dotndoh-checker/
    dotndoh-checker.exe
    DoT.txt
    DoH.txt
    README.md
    LICENSE
  dotndoh-checker-windows.zip
```

Полезные цели:

```powershell
make run      # запуск из исходников
make binary   # только бинарь в dist/
make package  # бинарь + списки + zip
make clean    # удалить dist/
```

Без `make` можно собрать напрямую:

```powershell
go build -trimpath -ldflags="-s -w" -o dist/dotndoh-checker.exe .
```

Версия берется из файла `VERSION`. При сборке через `make build` она также
попадает в имя архива и внутрь бинаря:

```powershell
.\dist\dotndoh-checker.exe -version
```

## CI и релизы

В проекте есть GitHub Actions workflow:

```text
.github/workflows/release.yml
```

На каждый push в `main` или `master` workflow:

1. Читает версию из `VERSION`.
2. Формирует тег `v{version}`, например `v1.0.0`.
3. Если такой тег уже существует, пропускает релиз.
4. Если тега еще нет, собирает Windows `amd64` бинарь.
5. Упаковывает в zip бинарь, `DoT.txt`, `DoH.txt`, `README.md`, `LICENSE` и `VERSION`.
6. Создает git tag и GitHub Release с архивом.

Чтобы выпустить новую версию, измените `VERSION`, например:

```text
1.0.1
```

затем сделайте commit и push.

## Запуск бинаря

После сборки:

```powershell
.\dist\dotndoh-checker.exe
```

Или из распакованного архива:

```powershell
.\dotndoh-checker.exe -count 10 -top 4
```

Важно: `DoT.txt` и `DoH.txt` должны лежать в текущей папке запуска, если вы не
передали свои пути через параметры.

## Параметры

```text
-dot string
    путь к списку DoT, по умолчанию DoT.txt

-doh string
    путь к списку DoH, по умолчанию DoH.txt

-domain string
    домен для тестового DNS-запроса, по умолчанию example.com

-count int
    количество попыток на каждый резолвер, по умолчанию 7

-timeout duration
    таймаут одной попытки, по умолчанию 3.5s

-top int
    сколько лучших резолверов показать, по умолчанию 4

-only string
    проверить только dot или только doh
```

## Формат списков

`DoT.txt`:

```text
name host [ip]
```

Примеры:

```text
Cloudflare-1 cloudflare-dns.com 1.1.1.1
Mullvad-Base base.dns.mullvad.net
```

Если `ip` указан, программа подключается к этому IP на порт `853`, но TLS/SNI
проверяет по `host`. Если `ip` не указан, программа сама резолвит `host` и
подключается к `host:853`.

`DoH.txt`:

```text
name url
```

Примеры:

```text
Cloudflare https://cloudflare-dns.com/dns-query
Google https://dns.google/dns-query
```

Пустые строки и строки с `#` в начале игнорируются.

## Как выбирать для роутера

Обычно стоит брать не только самый быстрый сервер, а несколько стабильных:

1. Сначала смотрите на `OK`, например `7/7`.
2. Потом на `Median`, потому что медиана меньше зависит от случайных всплесков.
3. Потом на `Max`, чтобы отсеять резолверы с редкими, но сильными задержками.
4. Для роутера удобно выбрать 2-4 endpoint'а одного протокола.

Если DoH заметно быстрее DoT, это нормально: маршрут, CDN и TLS/HTTP стек у
провайдера могут отличаться. Выбирайте тот протокол, который лучше поддерживает
ваш роутер.

## Примечания

Списки включают публичные бесплатные endpoint'ы с разными политиками: обычные,
security, family, ad blocking, unfiltered и другие. Перед постоянным
использованием проверьте, подходит ли конкретный профиль фильтрации под вашу
сеть.
