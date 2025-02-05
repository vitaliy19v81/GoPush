package main

import (
	"GoPush/config"
	"GoPush/db_postgres"
	_ "GoPush/docs"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	webpush "github.com/SherClockHolmes/webpush-go" // Внешняя библиотека для Web Push
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"time"

	//"github.com/redis/go-redis/v9"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"math/big"
	"net/http"
	//"github.com/jackc/pgx/v5/stdlib" // pgx-драйвер с поддержкой database/sql
	"github.com/gin-gonic/gin"
	_ "github.com/jackc/pgx/v5/stdlib"
	"log"
)

// go get -u github.com/swaggo/swag/cmd/swag
//go get -u github.com/swaggo/http-swagger
//go get -u github.com/swaggo/gin-swagger
//go mod tidy
//export PATH=$PATH:$(go env GOPATH)/bin
//swag init -g main.go -o ./docs

type TransferRequest struct {
	FromAccount string  `json:"from_account"`
	ToAccount   string  `json:"to_account"`
	Amount      float64 `json:"amount"`
}

type DepositRequest struct {
	UserID string  `json:"user_id"`
	Amount float64 `json:"amount"`
}

type WithdrawRequest struct {
	UserID string  `json:"user_id"`
	Amount float64 `json:"amount"`
}

type ConfirmationRequest struct {
	Code string `json:"code"`
}

type PushSubscription struct {
	Endpoint string            `json:"endpoint"`
	Keys     map[string]string `json:"keys"`
}

var (
	//generatedCode   string
	//pendingTransfer *TransferRequest
	//subscription *PushSubscription

	vapidPublicKey  string
	vapidPrivateKey string
)

var (
	ctx         = context.Background()
	redisClient *redis.Client
)

var dbHandler *db_postgres.DBHandler

// Подключение к Redis
func initRedis() {
	redisClient = redis.NewClient(&redis.Options{
		Addr:     "localhost:6379", // IP и порт Redis
		Password: "",               // Пароль (если есть)
		DB:       0,                // Используем 0-ю базу данных
	})

	// Проверяем подключение
	_, err := redisClient.Ping(ctx).Result()
	if err != nil {
		log.Fatalf("Ошибка подключения к Redis: %v", err)
	}
	fmt.Println("✅ Подключение к Redis успешно!")
}

func storeInRedis(key string, data interface{}, duration time.Duration) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return redisClient.Set(ctx, key, jsonData, duration).Err()
}

func getFromRedis(key string) (string, error) {
	jsonData, err := redisClient.Get(ctx, key).Result()
	if err != nil {
		return "", err
	}
	return jsonData, nil
}

func main() {
	config.LoadConfig()

	// Доступ к ключам теперь через config
	vapidPublicKey = config.VapidPublicKey
	vapidPrivateKey = config.VapidPrivateKey

	initRedis() // Подключаем Redis

	var err error
	// Подключение к базе данных
	dbHandler, err = db_postgres.InitDB()
	if err != nil {
		log.Fatalf("Database initialization failed: %v", err)
	}
	defer dbHandler.DB.Close()

	// Создание таблицы
	err = db_postgres.CreateAccountsTable(dbHandler.DB)
	if err != nil {
		log.Fatalf("Failed to create table: %v", err)
	}

	// Создание таблицы
	err = db_postgres.CreateTransactionsTable(dbHandler.DB)
	if err != nil {
		log.Fatalf("Failed to create table: %v", err)
	}

	router := gin.Default()

	// Настройка CORS (при необходимости)
	router.Use(func(c *gin.Context) {
		// Указываем конкретный домен, с которого разрешаем запросы
		//c.Writer.Header().Set("Access-Control-Allow-Origin", "*") // запросы со всех источников ("*"), что может быть
		// небезопасно, особенно если ваш сервис работает с конфиденциальными данными или требует авторизации.
		c.Writer.Header().Set("Access-Control-Allow-Origin", "http://localhost:8080") // Укажите ваш фронтенд-домен // "http://localhost:3000"
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")             // Для передачи cookie и авторизации
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
		c.Writer.Header().Set("Access-Control-Expose-Headers", "Authorization") // Разрешить клиентам видеть заголовок (authToken)

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})

	//// Основной маршрут
	//router.GET("/", func(c *gin.Context) {
	//	formHandler(c.Writer, c.Request)
	//})

	// Группа API
	api := router.Group("/api/push")
	{
		api.POST("/transfer", gin.WrapF(transferHandler))
		api.POST("/confirm", gin.WrapF(confirmHandler))
		api.POST("/subscribe", gin.WrapF(subscribeHandler))
		api.POST("/deposit", gin.WrapF(depositHandler))
		api.POST("/withdraw", gin.WrapF(withdrawHandler))
	}

	//// Статические файлы
	//router.StaticFile("/push_input", "static/push_input.html")
	//router.StaticFile("/sw.js", "static/sw.js")
	//router.Static("/static", "static")

	//// Swagger UI
	url := ginSwagger.URL("http://localhost:8083/swagger/doc.json") // Указываем путь к JSON-документации

	router.GET("/swagger/*any", func(c *gin.Context) {
		// Если запрос на корневой путь /swagger/, редиректим на /swagger/index.html
		if c.Param("any") == "" || c.Param("any") == "/" {
			c.Redirect(http.StatusMovedPermanently, "/swagger/index.html")
			return
		}
		// Все остальные запросы обрабатывает Swagger Handler
		ginSwagger.WrapHandler(swaggerFiles.Handler, url)(c)
	}) //router.GET("/swagger/*any", gin.WrapH(httpSwagger.WrapHandler))

	log.Println("Сервер запущен на http://localhost:8083")
	log.Fatal(router.Run(":8083"))

}

func CORSMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")         // Разрешенный источник // "http://localhost:3000"
		w.Header().Set("Access-Control-Allow-Credentials", "true") // Для работы с cookie
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

//// Отображение формы перевода
//func formHandler(w http.ResponseWriter, r *http.Request) {
//	http.ServeFile(w, r, "static/transfer_form.html")
//}

// Сохранение подписки пользователя для push-уведомлений
func subscribeHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Неверный метод запроса", http.StatusMethodNotAllowed)
		return
	}

	var sub PushSubscription
	if err := json.NewDecoder(r.Body).Decode(&sub); err != nil {
		http.Error(w, "Некорректные данные подписки", http.StatusBadRequest)
		return
	}

	////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
	//subscription = &sub
	err := storeInRedis("user_subscription", sub, 5*time.Minute)
	if err != nil {
		log.Printf("Ошибка сохранения подписки в Redis: %v", err)
		http.Error(w, "Ошибка сервера", http.StatusInternalServerError)
		return
	}
	////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

	log.Printf("Подписка сохранена")
	w.WriteHeader(http.StatusOK)
}

// @Summary Пополнение счета
// @Description Пополняет баланс указанного аккаунта
// @Accept json
// @Produce json
// @Param deposit body DepositRequest true "Данные пополнения"
// @Success 200 {string} string "Пополнение успешно"
// @Failure 400 {string} string "Некорректные данные запроса"
// @Failure 500 {string} string "Ошибка пополнения счета"
// @Router /api/push/deposit [post]
func depositHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Неверный метод запроса", http.StatusMethodNotAllowed)
		return
	}

	var deposit DepositRequest
	if err := json.NewDecoder(r.Body).Decode(&deposit); err != nil {
		http.Error(w, "Некорректные данные запроса", http.StatusBadRequest)
		return
	}

	userUUID, err := uuid.Parse(deposit.UserID)
	if err != nil {
		http.Error(w, "Неверный формат UUID", http.StatusBadRequest)
		return
	}

	err = dbHandler.DepositFunds(userUUID, deposit.Amount)
	if err != nil {
		http.Error(w, "Ошибка пополнения счета", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Пополнение успешно"))
}

// @Summary Снятие средств
// @Description Позволяет пользователю снять средства со счета
// @Tags accounts
// @Accept json
// @Produce json
// @Param request body WithdrawRequest true "Данные для снятия средств"
// @Success 200 {string} string "Снятие успешно"
// @Failure 400 {string} string "Некорректные данные запроса"
// @Failure 500 {string} string "Ошибка снятия средств"
// @Router /api/push/withdraw [post]а
func withdrawHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Неверный метод запроса", http.StatusMethodNotAllowed)
		return
	}

	var withdraw WithdrawRequest
	if err := json.NewDecoder(r.Body).Decode(&withdraw); err != nil {
		http.Error(w, "Некорректные данные запроса", http.StatusBadRequest)
		return
	}

	userUUID, err := uuid.Parse(withdraw.UserID)
	if err != nil {
		http.Error(w, "Неверный формат UUID", http.StatusBadRequest)
		return
	}

	err = dbHandler.WithdrawFunds(userUUID, withdraw.Amount)
	if err != nil {
		http.Error(w, "Ошибка пополнения счета", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Снятие успешно"))
}

// @Summary Перевод средств
// @Description Осуществляет перевод средств между счетами
// @Tags transactions
// @Accept json
// @Produce json
// @Param request body TransferRequest true "Данные для перевода"
// @Success 200 {string} string "Push-уведомление отправлено"
// @Failure 400 {string} string "Некорректные данные запроса"
// @Failure 500 {string} string "Ошибка перевода"
// @Router /api/push/transfer [post]
func transferHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Неверный метод запроса", http.StatusMethodNotAllowed)
		return
	}

	var transfer TransferRequest
	if err := json.NewDecoder(r.Body).Decode(&transfer); err != nil {
		http.Error(w, "Некорректные данные запроса", http.StatusBadRequest)
		return
	}

	////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
	//pendingTransfer = &transfer
	err := storeInRedis("user_transfer", transfer, 5*time.Minute)
	if err != nil {
		log.Printf("Ошибка сохранения перевода в Redis: %v", err)
		http.Error(w, "Ошибка сервера", http.StatusInternalServerError)
		return
	}
	////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

	n, _ := rand.Int(rand.Reader, big.NewInt(1000000))
	generatedCode := fmt.Sprintf("%06d", n.Int64())
	log.Printf("Сгенерированный код подтверждения: %s", generatedCode)

	// Подготовка данных в формате JSON
	data := map[string]string{"code": generatedCode}

	////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
	err = storeInRedis("generated_code", data, 5*time.Minute)
	if err != nil {
		log.Printf("Ошибка сохранения кода в Redis: %v", err)
		http.Error(w, "Ошибка сервера", http.StatusInternalServerError)
		return
	}
	////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

	////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
	jsonSubscription, err := getFromRedis("user_subscription")
	if err != nil {
		log.Printf("Ошибка при получении подписки из Redis: %v", err)
		http.Error(w, "Ошибка сервера", http.StatusInternalServerError)
		return
	}
	////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

	var subscription PushSubscription
	err = json.Unmarshal([]byte(jsonSubscription), &subscription)
	if err != nil {
		log.Printf("Ошибка при десериализации данных: %v", err)
		http.Error(w, "Ошибка сервера", http.StatusInternalServerError)
		return
	}

	// Отправка push-уведомления
	if subscription.Endpoint != "" {
		notification := fmt.Sprintf("Ваш код подтверждения: %s", generatedCode)
		sendPushNotification(&subscription, notification)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Push-уведомление отправлено"))
	} else {
		http.Error(w, "Подписка не найдена", http.StatusBadRequest)
	}
}

// Обработка подтверждения перевода
func confirmHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Неверный метод запроса", http.StatusMethodNotAllowed)
		return
	}

	var confirmation ConfirmationRequest
	if err := json.NewDecoder(r.Body).Decode(&confirmation); err != nil {
		http.Error(w, "Некорректные данные запроса", http.StatusBadRequest)
		return
	}

	////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
	jsonCode, err := getFromRedis("generated_code")
	if err != nil {
		log.Printf("Ошибка при получении кода из redis: %v", err)
		http.Error(w, "Ошибка сервера", http.StatusInternalServerError)
		return
	}
	////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

	// Распаковка JSON в карту
	var result map[string]string
	err = json.Unmarshal([]byte(jsonCode), &result)
	if err != nil {
		log.Printf("Ошибка при десериализации данных из Redis: %v", err)
		http.Error(w, "Ошибка сервера", http.StatusInternalServerError)
		return
	}

	// Извлечение значения по ключу "code"
	generatedCode, exists := result["code"]
	if !exists {
		log.Println("Ключ 'code' не найден в Redis")
		http.Error(w, "Ошибка сервера", http.StatusInternalServerError)
		return
	}

	////////////////////////////////////////////////////////////////////////////////////////////////////////////////////
	jsonTransfer, err := getFromRedis("user_transfer")
	if err != nil {
		log.Printf("Ошибка при получении перевода из Redis: %v", err)
		http.Error(w, "Ошибка сервера", http.StatusInternalServerError)
		return
	}
	////////////////////////////////////////////////////////////////////////////////////////////////////////////////////

	var pendingTransfer TransferRequest
	err = json.Unmarshal([]byte(jsonTransfer), &pendingTransfer)
	if err != nil {
		log.Printf("Ошибка при десериализации данных: %v", err)
		http.Error(w, "Ошибка сервера", http.StatusInternalServerError)
		return
	}

	if confirmation.Code == generatedCode && pendingTransfer.Amount != 0 {
		// Вызов функции для выполнения транзакции
		err := executeTransaction(&pendingTransfer)
		if err != nil {
			log.Printf("Ошибка при выполнении перевода: %v", err)
			http.Error(w, "Ошибка при выполнении перевода", http.StatusInternalServerError)
			return
		}

		log.Printf("Перевод успешно выполнен: %v", pendingTransfer)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Перевод успешно выполнен"))
		//pendingTransfer = nil
	} else {
		http.Error(w, "Неверный код подтверждения", http.StatusUnauthorized)
	}
}

// Функция отправки push-уведомлений
func sendPushNotification(sub *PushSubscription, message string) {
	resp, err := webpush.SendNotification([]byte(message), &webpush.Subscription{
		Endpoint: sub.Endpoint,
		Keys: webpush.Keys{
			P256dh: sub.Keys["p256dh"],
			Auth:   sub.Keys["auth"],
		},
	}, &webpush.Options{
		Subscriber:      "mailto:example@yourdomain.com",
		VAPIDPublicKey:  vapidPublicKey,
		VAPIDPrivateKey: vapidPrivateKey,
		TTL:             30,
	})
	if err != nil {
		log.Printf("Ошибка при отправке push-уведомления: %v", err)
		return
	}
	defer resp.Body.Close()
	log.Println("Push-уведомление успешно отправлено")
}

// Перевод средств с аккаунта на аккаунт
func executeTransaction(transfer *TransferRequest) error {

	// Преобразуем строки в UUID
	fromAccountUUID, err := uuid.Parse(transfer.FromAccount)
	if err != nil {
		return fmt.Errorf("Неверный формат UUID для отправителя: %v", err)
	}

	toAccountUUID, err := uuid.Parse(transfer.ToAccount)
	if err != nil {
		return fmt.Errorf("Неверный формат UUID для получателя: %v", err)
	}

	// Логика перевода средств
	log.Printf("Перевод с аккаунта %s на аккаунт %s на сумму %.2f", fromAccountUUID, toAccountUUID, transfer.Amount)

	// Вызов функции для перевода средств
	err = dbHandler.TransferFunds(fromAccountUUID, toAccountUUID, transfer.Amount)
	if err != nil {
		return fmt.Errorf("Ошибка при выполнении перевода: %v", err)
	}

	return nil // Возвращаем nil, если транзакция прошла успешно
}

//mux := http.NewServeMux()
//
//// Основной маршрут
//mux.HandleFunc("/", formHandler)
//
//// Группа API
//apiMux := http.NewServeMux()
//apiMux.HandleFunc("/transfer", transferHandler)
//apiMux.HandleFunc("/confirm", confirmHandler)
//apiMux.HandleFunc("/subscribe", subscribeHandler)
//apiMux.HandleFunc("/deposit", depositHandler)
//apiMux.HandleFunc("/withdraw", withdrawHandler)
//
//// Встраиваем API маршруты в основной
//mux.Handle("/api/", http.StripPrefix("/api/push", apiMux))
//
//// Статические файлы
//mux.Handle("/push_input", http.FileServer(http.Dir("static")))
//mux.Handle("/sw.js", http.StripPrefix("/", http.FileServer(http.Dir("static"))))
//mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
//mux.Handle("/swagger/", httpSwagger.WrapHandler) // Swagger UI
//
//// Оборачиваем в CORS middleware
//handler := CORSMiddleware(mux)
//
//log.Println("Сервер запущен на http://localhost:8083")
//log.Fatal(http.ListenAndServe(":8083", handler))

//http.HandleFunc("/", formHandler)
//http.HandleFunc("/api/transfer", transferHandler)
//http.HandleFunc("/api/confirm", confirmHandler)
//http.HandleFunc("/api/subscribe", subscribeHandler)
//http.HandleFunc("/api/deposit", depositHandler)
//http.HandleFunc("/api/withdraw", withdrawHandler)
//http.HandleFunc("/push_input", func(w http.ResponseWriter, r *http.Request) {
//	http.ServeFile(w, r, "static/push_input.html")
//})
//http.Handle("/sw.js", http.StripPrefix("/", http.FileServer(http.Dir("static"))))
////http.Handle("/sw.js", http.FileServer(http.Dir("static")))
//
//http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
//http.Handle("/swagger/", httpSwagger.WrapHandler) // Подключаем Swagger UI
//
//// Оборачиваем http.DefaultServeMux в CORS middleware
//handler := CORSMiddleware(http.DefaultServeMux) // middleware.
//
//log.Println("Сервер запущен на http://localhost:8083")
//log.Fatal(http.ListenAndServe(":8083", handler))
