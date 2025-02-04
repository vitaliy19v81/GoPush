package main

import (
	"GoPush/config"
	"GoPush/db_postgres"
	_ "GoPush/docs"
	"crypto/rand"
	"encoding/json"
	"fmt"
	webpush "github.com/SherClockHolmes/webpush-go" // Внешняя библиотека для Web Push
	"github.com/google/uuid"
	_ "github.com/lib/pq"
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
	generatedCode   string
	pendingTransfer *TransferRequest
	subscription    *PushSubscription

	vapidPublicKey  string
	vapidPrivateKey string
)

var dbHandler *db_postgres.DBHandler

//var (
//	redisClient *redis.Client
//	ctx         = context.Background()
//)
//
//// Подключение к Redis
//func initRedis() {
//	redisClient = redis.NewClient(&redis.Options{
//		Addr:     "localhost:6379", // IP и порт Redis
//		Password: "",               // Если нет пароля, оставить пустым
//		DB:       0,                // Использовать 0-ю базу данных
//	})
//
//	// Проверка соединения
//	_, err := redisClient.Ping(ctx).Result()
//	if err != nil {
//		log.Fatalf("Ошибка подключения к Redis: %v", err)
//	}
//	log.Println("Подключение к Redis успешно!")
//}
//
//// Функция сохранения данных в Redis
//func saveToRedis() {
//	// Записываем generatedCode (обычная строка)
//	err := redisClient.Set(ctx, "generatedCode", "123456", 10*time.Minute).Err()
//	if err != nil {
//		log.Printf("Ошибка сохранения generatedCode в Redis: %v", err)
//	}
//
//	// Записываем pendingTransfer (JSON)
//	pending := TransferRequest{ID: "TRX123", Amount: 100.50}
//	pendingJSON, _ := json.Marshal(pending)
//	err = redisClient.Set(ctx, "pendingTransfer", pendingJSON, 10*time.Minute).Err()
//	if err != nil {
//		log.Printf("Ошибка сохранения pendingTransfer в Redis: %v", err)
//	}
//
//	// Записываем subscription (JSON)
//	sub := PushSubscription{
//		Endpoint: "https://example.com/endpoint",
//		Keys: map[string]string{
//			"p256dh": "key1",
//			"auth":   "key2",
//		},
//	}
//	subJSON, _ := json.Marshal(sub)
//	err = redisClient.Set(ctx, "subscription", subJSON, 10*time.Minute).Err()
//	if err != nil {
//		log.Printf("Ошибка сохранения subscription в Redis: %v", err)
//	}
//}
//
//// Функция загрузки данных из Redis
//func loadFromRedis() {
//	// Читаем generatedCode
//	generatedCode, err := redisClient.Get(ctx, "generatedCode").Result()
//	if errors.Is(err, redis.Nil) {
//		log.Println("generatedCode не найден в Redis")
//	} else if err != nil {
//		log.Printf("Ошибка загрузки generatedCode: %v", err)
//	} else {
//		log.Println("Загруженный generatedCode:", generatedCode)
//	}
//
//	// Читаем pendingTransfer
//	pendingJSON, err := redisClient.Get(ctx, "pendingTransfer").Result()
//	if errors.Is(err, redis.Nil) {
//		log.Println("pendingTransfer не найден в Redis")
//	} else if err != nil {
//		log.Printf("Ошибка загрузки pendingTransfer: %v", err)
//	} else {
//		var pending TransferRequest
//		json.Unmarshal([]byte(pendingJSON), &pending)
//		log.Println("Загруженный pendingTransfer:", pending)
//	}
//
//	// Читаем subscription
//	subJSON, err := redisClient.Get(ctx, "subscription").Result()
//	if errors.Is(err, redis.Nil) {
//		log.Println("subscription не найден в Redis")
//	} else if err != nil {
//		log.Printf("Ошибка загрузки subscription: %v", err)
//	} else {
//		var sub PushSubscription
//		json.Unmarshal([]byte(subJSON), &sub)
//		log.Println("Загруженный subscription:", sub)
//	}
//}

func main() {
	config.LoadConfig()

	// Доступ к ключам теперь через config
	vapidPublicKey = config.VapidPublicKey
	vapidPrivateKey = config.VapidPrivateKey

	//initRedis()     // Подключаем Redis
	//saveToRedis()   // Сохраняем данные в Redis
	//loadFromRedis() // Читаем данные из Redis

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

	subscription = &sub
	log.Printf("Подписка сохранена: %+v", subscription)
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

	pendingTransfer = &transfer

	n, _ := rand.Int(rand.Reader, big.NewInt(1000000))
	generatedCode = fmt.Sprintf("%06d", n.Int64())
	log.Printf("Сгенерированный код подтверждения: %s", generatedCode)

	// Отправка push-уведомления
	if subscription != nil {
		notification := fmt.Sprintf("Ваш код подтверждения: %s", generatedCode)
		sendPushNotification(subscription, notification)
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

	if confirmation.Code == generatedCode && pendingTransfer != nil {
		log.Printf("Информация: %v", pendingTransfer)
		// Вызов функции для выполнения транзакции
		err := executeTransaction(pendingTransfer)
		if err != nil {
			log.Printf("Ошибка при выполнении перевода: %v", err)
			http.Error(w, "Ошибка при выполнении перевода", http.StatusInternalServerError)
			return
		}

		log.Printf("Перевод успешно выполнен: %v", pendingTransfer)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("Перевод успешно выполнен"))
		pendingTransfer = nil
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

////package main
////
////import (
////	"encoding/json"
////	"log"
////	"net/http"
////)
////
////func formHandler(w http.ResponseWriter, r *http.Request) {
////	http.ServeFile(w, r, "static/transfer_form.html")
////}
////
////func pushInputHandler(w http.ResponseWriter, r *http.Request) {
////	http.ServeFile(w, r, "static/push_input.html")
////}
////
////func subscribeHandler(w http.ResponseWriter, r *http.Request) {
////	var sub PushSubscription
////	err := json.NewDecoder(r.Body).Decode(&sub)
////	if err != nil {
////		http.Error(w, "Invalid subscription", http.StatusBadRequest)
////		return
////	}
////	// Тут должна быть логика сохранения подписки в базе данных или другом хранилище
////	log.Printf("Subscription saved: %+v\n", sub)
////	w.WriteHeader(http.StatusOK)
////}
////
////func transferHandler(w http.ResponseWriter, r *http.Request) {
////	if r.Method == "POST" {
////		var data TransferRequest
////		err := json.NewDecoder(r.Body).Decode(&data)
////		if err != nil {
////			http.Error(w, "Invalid request body", http.StatusBadRequest)
////			return
////		}
////
////		// Логика обработки перевода
////
////		// Отправка уведомления через push
////		sendPushNotification(data)
////		w.WriteHeader(http.StatusOK)
////	}
////}
////
////func sendPushNotification(data TransferRequest) {
////	// Тут логика отправки push уведомления
////	// Например, можно использовать библиотеку для работы с Web Push
////	log.Println("Sending push notification:", data)
////}
////
////func main() {
////	http.HandleFunc("/", formHandler)
////	http.HandleFunc("/push_input", pushInputHandler)
////	http.HandleFunc("/api/subscribe", subscribeHandler)
////	http.HandleFunc("/api/transfer", transferHandler)
////
////	log.Println("Server started on :8080")
////	http.ListenAndServe(":8080", nil)
////}
//
////type PushSubscription struct {
////	Endpoint string `json:"endpoint"`
////	Keys     struct {
////		Auth   string `json:"auth"`
////		P256dh string `json:"p256dh"`
////	} `json:"keys"`
////}
////
////type TransferRequest struct {
////	FromAccount string  `json:"from_account"`
////	ToAccount   string  `json:"to_account"`
////	Amount      float64 `json:"amount"`
////}
//
//package main
//
//import (
//	"encoding/json"
//	"fmt"
//	"log"
//	"math/rand"
//	"net/http"
//	_ "strconv"
//	"time"
//
//	webpush "github.com/SherClockHolmes/webpush-go" // Внешняя библиотека для Web Push
//)
//
//type TransferRequest struct {
//	FromAccount string  `json:"from_account"`
//	ToAccount   string  `json:"to_account"`
//	Amount      float64 `json:"amount"`
//}
//
//type ConfirmationRequest struct {
//	Code string `json:"code"`
//}
//
//type PushSubscription struct {
//	Endpoint string            `json:"endpoint"`
//	Keys     map[string]string `json:"keys"`
//}
//
//var (
//	generatedCode   string
//	pendingTransfer *TransferRequest
//	subscription    *PushSubscription
//	vapidPublicKey  = "BI2izS49Rqe339JP7w4qS214CbUusG2VCbIhSStOtFBj7Va-GFi0kGfv21_A8cS1OASvib8GdbIlBs1ZJYc9JVw"
//	vapidPrivateKey = "epgdmo6gxfcxxuQZ2PiLziudajnBu5jGEDBqm_GGtZE"
//)
//
//func main() {
//	http.HandleFunc("/", formHandler)
//	http.HandleFunc("/api/transfer", transferHandler)
//	http.HandleFunc("/api/confirm", confirmHandler)
//	http.HandleFunc("/api/subscribe", subscribeHandler)
//	http.HandleFunc("/push_input", func(w http.ResponseWriter, r *http.Request) {
//		http.ServeFile(w, r, "static/push_input.html")
//	})
//	http.Handle("/sw.js", http.StripPrefix("/", http.FileServer(http.Dir("static"))))
//	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
//
//	log.Println("Server running on http://localhost:8080")
//	log.Fatal(http.ListenAndServe(":8080", nil))
//}
//
//// Отображение начальной формы
//func formHandler(w http.ResponseWriter, r *http.Request) {
//	http.ServeFile(w, r, "static/transfer_form.html")
//}
//
//// Сохранение подписки пользователя
//func subscribeHandler(w http.ResponseWriter, r *http.Request) {
//	if r.Method != http.MethodPost {
//		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
//		return
//	}
//
//	var sub PushSubscription
//	if err := json.NewDecoder(r.Body).Decode(&sub); err != nil {
//		http.Error(w, "Invalid subscription data", http.StatusBadRequest)
//		return
//	}
//
//	subscription = &sub
//	log.Printf("Subscription saved: %+v", subscription)
//	w.WriteHeader(http.StatusOK)
//}
//
//// Обработка запроса на перевод
//func transferHandler(w http.ResponseWriter, r *http.Request) {
//	if r.Method != http.MethodPost {
//		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
//		return
//	}
//
//	var transfer TransferRequest
//	if err := json.NewDecoder(r.Body).Decode(&transfer); err != nil {
//		http.Error(w, "Invalid request data", http.StatusBadRequest)
//		return
//	}
//
//	pendingTransfer = &transfer
//
//	// Генерация 6-значного кода
//	rand.Seed(time.Now().UnixNano())
//	generatedCode = fmt.Sprintf("%06d", rand.Intn(1000000))
//	log.Printf("Generated code: %s", generatedCode)
//
//	// Отправка push-уведомления
//	if subscription != nil {
//		notification := fmt.Sprintf("Your confirmation code is: %s", generatedCode)
//		sendPushNotification(subscription, notification)
//		w.WriteHeader(http.StatusOK)
//		w.Write([]byte("Push notification sent"))
//	} else {
//		http.Error(w, "No subscription found", http.StatusBadRequest)
//	}
//}
//
//// Подтверждение перевода
//func confirmHandler(w http.ResponseWriter, r *http.Request) {
//	if r.Method != http.MethodPost {
//		http.Error(w, "Invalid request method", http.StatusMethodNotAllowed)
//		return
//	}
//
//	var confirmation ConfirmationRequest
//	if err := json.NewDecoder(r.Body).Decode(&confirmation); err != nil {
//		http.Error(w, "Invalid request data", http.StatusBadRequest)
//		return
//	}
//
//	if confirmation.Code == generatedCode && pendingTransfer != nil {
//		log.Printf("Transfer successful: %v", pendingTransfer)
//		w.WriteHeader(http.StatusOK)
//		w.Write([]byte("Transfer successful"))
//		pendingTransfer = nil
//	} else {
//		http.Error(w, "Invalid code", http.StatusUnauthorized)
//	}
//}
//
//// Отправка push-уведомления
//func sendPushNotification(sub *PushSubscription, message string) {
//	// Создаем уведомление
//	resp, err := webpush.SendNotification([]byte(message), &webpush.Subscription{
//		Endpoint: sub.Endpoint,
//		Keys: webpush.Keys{
//			P256dh: sub.Keys["p256dh"],
//			Auth:   sub.Keys["auth"],
//		},
//	}, &webpush.Options{
//		Subscriber:      "mailto:example@yourdomain.com",
//		VAPIDPublicKey:  vapidPublicKey,
//		VAPIDPrivateKey: vapidPrivateKey,
//		TTL:             30,
//	})
//	if err != nil {
//		log.Printf("Error sending push notification: %v", err)
//		return
//	}
//	defer resp.Body.Close()
//	log.Println("Push notification sent successfully")
//}
