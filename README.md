<div align="center">

# 🤖 FlorenBot

**Открытый (Open Source) игровой бот на Go для развлечения в групповых чатах и беседах Telegram**

[![License](https://img.shields.io/badge/License-GPL_v3-blue.svg)](LICENSE)
[![Language](https://img.shields.io/badge/Language-Go_1.27+-00ADD8?logo=go&logoColor=white)](https://go.dev/)
[![Platform](https://img.shields.io/badge/Platform-Telegram-26A5E4?logo=telegram&logoColor=white)](https://core.telegram.org/bots)

---

### 🌐 Сообщество и ресурсы

[![Releases](https://img.shields.io/badge/GitHub-Releases-181717?style=for-the-badge&logo=github&logoColor=white)](https://github.com/FlorenBot/florenbot/releases)
[![Telegram Channel](https://img.shields.io/badge/Telegram-Канал_Dev-26A5E4?style=for-the-badge&logo=telegram&logoColor=white)](https://t.me/florenbotdev)
[![Reddit](https://img.shields.io/badge/Reddit-r%2Fflorenbot-FF4500?style=for-the-badge&logo=reddit&logoColor=white)](https://www.reddit.com/r/florenbot)
[![Patreon](https://img.shields.io/badge/Patreon-Поддержать-FF424D?style=for-the-badge&logo=patreon&logoColor=white)](https://www.patreon.com/c/egorluchiy2026)
</div>

---

## 🔥 Преимущества

1. **Регулярные обновления:** Бот активно развивается и пополняется новым функционалом.
2. **Высокая скорость и комфорт:** Мгновенный отклик на команды благодаря оптимизированному Go-коду.
3. **Open Source:** Вы можете запустить собственного бота на базе нашего исходного кода.

---

## 🛠️ Архитектура и стек

* **Язык:** Go (Golang) — обеспечивает высокую скорость обработки запросов и эффективную многопоточность через горутины.
* **База данных (MySQL / MariaDB):** Надежное хранение профилей игроков и игрового прогресса.
* **Кэш (Redis):** Хранение сессий, состояний в реальном времени и минимизация нагрузки на SQL.

---

## ✨ Основные возможности

* **Быстрая авторизация:** Проверка данных через Redis с последующей асинхронной синхронизацией с MySQL.
* **Игровой цикл:** Обработка событий и игровых команд в реальном времени.
* **Масштабируемость:** Способность обрабатывать тысячи одновременных запросов в чатах.

---

## 📋 Требования к запуску

* **Go** 1.22+
* **MySQL** 8.0 / **MariaDB** 11.0+
* **Redis** 7.0+
* **Docker** & **Docker Compose**

---

## 🚀 Установка и запуск

### 1. Клонируйте репозиторий:

```bash
    git clone https://github.com/EgorTienda/florenbot.git
    cd florenbot
 ```

2. **Настройка окружения:**
   Скопируйте `.env.docker.example` в `.env` и укажите параметры подключения к базам данных:
   ```env
    REDIS_ADDR=localhost:6379
    REDIS_PASSWORD=
    REDIS_DB=0
    
    DB_HOST=localhost
    DB_PORT=3306
    DB_USER=
    DB_PASSWORD=
    DB_NAME=
    DB_ROOT_PASSWORD=
   
   ```

## 3. Запустите проект

```bash
    docker compose up -d
```

## Важно!

Не забывайте про **BOT_TOKEN** его нужно подставить в @BotFather вам выдает Токен бота и вы должны ставить туда.

## Инструкции к билду на Debian 13:

https://github.com/FlorenBot/florenbot/blob/main/BUILDING.md
