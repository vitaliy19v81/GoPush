// /GoPush/db_postgres/db.go
package db_postgres

import (
	"GoPush/config"
	"database/sql"
	"errors"
	"fmt"
	"github.com/google/uuid"
	_ "github.com/lib/pq"
	"log"

	//"github.com/jackc/pgx/v5/stdlib" // pgx-драйвер с поддержкой database/sql
	_ "github.com/jackc/pgx/v5/stdlib"
)

// go get github.com/lib/pq  # для pq
//go get github.com/jackc/pgx/v5  # для pgx

type DBHandler struct {
	DB *sql.DB
}

func InitDB() (*DBHandler, error) {
	dsn := config.DsnPostgres
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &DBHandler{DB: db}, nil
}

func CreateAccountsTable(db *sql.DB) error {
	query := `
	CREATE EXTENSION IF NOT EXISTS "pgcrypto"; -- Подключение расширения для генерации UUID
	CREATE TABLE IF NOT EXISTS accounts (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    	user_id UUID NOT NULL UNIQUE,
    	balance DECIMAL(10, 2) NOT NULL DEFAULT 0,
    	updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
	    CONSTRAINT fk_user FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_accounts_user_id ON accounts(user_id);
	`
	_, err := db.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to create accounts table: %w", err)
	}
	return nil
}

func CreateTransactionsTable(db *sql.DB) error {
	query := `
	CREATE EXTENSION IF NOT EXISTS "pgcrypto"; -- Подключение расширения для генерации UUID
	CREATE TABLE IF NOT EXISTS transactions (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		from_account UUID NOT NULL,
		to_account UUID NOT NULL,
		amount DECIMAL(10, 2) NOT NULL,
		status VARCHAR(20) NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    	CONSTRAINT fk_from_account FOREIGN KEY (from_account) REFERENCES accounts(user_id) ON DELETE CASCADE,
    	CONSTRAINT fk_to_account FOREIGN KEY (to_account) REFERENCES accounts(user_id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_from_account ON transactions(from_account);
	CREATE INDEX IF NOT EXISTS idx_to_account ON transactions(to_account);
	`
	_, err := db.Exec(query)
	if err != nil {
		return fmt.Errorf("failed to create transactions table: %w", err)
	}
	return nil
}

// Проверка достаточности баланса
func (db *DBHandler) CheckSufficientFunds(userID uuid.UUID, amount float64) (bool, error) {
	var balance float64
	err := db.DB.QueryRow("SELECT balance FROM accounts WHERE user_id = $1", userID).Scan(&balance)
	if err != nil {
		return false, fmt.Errorf("ошибка при получении баланса: %v", err)
	}

	if balance < amount {
		return false, nil // Недостаточно средств
	}

	return true, nil // Достаточно средств
}

// DepositFunds Внесение денежной суммы в кассу
func (db *DBHandler) DepositFunds(userID uuid.UUID, amount float64) error {
	if amount <= 0 {
		return fmt.Errorf("сумма должна быть положительной")
	}

	//tx, err := db.DB.Begin()
	//if err != nil {
	//	return fmt.Errorf("ошибка при начале транзакции: %v", err)
	//}
	//
	//// Пополняем баланс счета
	//_, err = tx.Exec("UPDATE accounts SET balance = balance + $1 WHERE id = $2", amount, accountID)
	//if err != nil {
	//	tx.Rollback()
	//	return fmt.Errorf("ошибка при пополнении счета: %v", err)
	//}
	//
	//// Записываем транзакцию депозита
	//_, err = tx.Exec("INSERT INTO transactions (from_account, to_account, amount, status) VALUES ($1, $2, $3, $4)",
	//	nil, accountID, amount, "deposit")
	//if err != nil {
	//	tx.Rollback()
	//	return fmt.Errorf("ошибка при записи транзакции депозита: %v", err)
	//}
	//
	//err = tx.Commit()
	//if err != nil {
	//	return fmt.Errorf("ошибка при завершении транзакции депозита: %v", err)
	//}
	//
	//return nil

	tx, err := db.DB.Begin()
	if err != nil {
		return fmt.Errorf("ошибка начала транзакции: %v", err)
	}

	// 1. Проверяем, есть ли аккаунт у пользователя
	var accountID uuid.UUID
	err = tx.QueryRow("SELECT id FROM accounts WHERE user_id = $1", userID).Scan(&accountID)

	if errors.Is(err, sql.ErrNoRows) {
		// 2. Если аккаунта нет — создаем его с балансом 0
		err = tx.QueryRow(
			"INSERT INTO accounts (user_id, balance) VALUES ($1, 0) RETURNING id",
			userID,
		).Scan(&accountID)
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("ошибка создания аккаунта: %v", err)
		}
	} else if err != nil {
		tx.Rollback()
		return fmt.Errorf("ошибка при поиске аккаунта: %v", err)
	}

	// 3. Обновляем баланс
	_, err = tx.Exec("UPDATE accounts SET balance = balance + $1 WHERE id = $2", amount, accountID)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("ошибка пополнения счета: %v", err)
	}

	// 4. Фиксируем транзакцию
	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("ошибка при завершении транзакции: %v", err)
	}

	return nil
}

// WithdrawFunds Снятие суммы со счета
func (db *DBHandler) WithdrawFunds(userID uuid.UUID, amount float64) error {
	if amount <= 0 {
		return fmt.Errorf("сумма снятия должна быть положительной")
	}

	sufficient, err := db.CheckSufficientFunds(userID, amount)
	if err != nil {
		return fmt.Errorf("ошибка проверки средств: %v", err)
	}

	if !sufficient {
		return fmt.Errorf("недостаточно средств на счете")
	}

	tx, err := db.DB.Begin()
	if err != nil {
		return fmt.Errorf("ошибка при начале транзакции: %v", err)
	}

	_, err = tx.Exec("UPDATE accounts SET balance = balance - $1 WHERE user_id = $2", amount, userID)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("ошибка при обновлении баланса: %v", err)
	}

	_, err = tx.Exec("INSERT INTO transactions (from_account, to_account, amount, status) VALUES ($1, $2, $3, $4)",
		userID, nil, amount, "withdrawal")
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("ошибка при записи транзакции: %v", err)
	}

	if err = tx.Commit(); err != nil {
		return fmt.Errorf("ошибка при завершении транзакции: %v", err)
	}

	return nil
}

// TransferFunds Перевод средств с счета на счет
func (db *DBHandler) TransferFunds(fromAccount, toAccount uuid.UUID, amount float64) error {
	// 1. Проверяем, достаточно ли средств
	sufficient, err := db.CheckSufficientFunds(fromAccount, amount)
	if err != nil {
		return fmt.Errorf("ошибка проверки средств: %v", err)
	}

	if !sufficient {
		return fmt.Errorf("недостаточно средств на счете отправителя")
	}

	// 2. Переводим деньги
	tx, err := db.DB.Begin() // Начинаем транзакцию
	if err != nil {
		return fmt.Errorf("ошибка при начале транзакции: %v", err)
	}

	// Уменьшаем баланс отправителя
	_, err = tx.Exec("UPDATE accounts SET balance = balance - $1 WHERE user_id = $2", amount, fromAccount)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("ошибка при обновлении баланса отправителя: %v", err)
	}

	// Увеличиваем баланс получателя
	_, err = tx.Exec("UPDATE accounts SET balance = balance + $1 WHERE user_id = $2", amount, toAccount)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("ошибка при обновлении баланса получателя: %v", err)
	}

	// 3. Записываем транзакцию
	log.Printf("Записываем транзакцию: from=%s, to=%s, amount=%.2f", fromAccount, toAccount, amount)
	_, err = tx.Exec("INSERT INTO transactions (from_account, to_account, amount, status) VALUES ($1, $2, $3, $4)",
		fromAccount, toAccount, amount, "completed")
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("ошибка при записи транзакции: %v", err)
	}

	// Фиксируем транзакцию
	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("ошибка при завершении транзакции: %v", err)
	}

	return nil
}
