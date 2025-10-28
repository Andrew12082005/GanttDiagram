package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	// *** 修改 1：移除 CGO 驅動 ***
	// _ "github.com/mattn/go-sqlite3" 

	// *** 修改 2：匯入 Pure Go 驅動 ***
	_ "modernc.org/sqlite"
)

// --- 1. 數據結構 (與前端/JSON 標籤匹配) ---
type Task struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Start        string `json:"start"` // YYYY-MM-DD 格式
	DurationDays int    `json:"durationDays"`
	Color        string `json:"color"`
	Priority     int    `json:"priority"`
}

// --- 2. 資料庫操作 ---

// 全局資料庫連接
var db *sql.DB

// initDB 初始化資料庫連接並創建資料表 (如果不存在)
func initDB(filepath string) (*sql.DB, error) {
	var err error
	
	// *** 修改 3：將 "sqlite3" 改為 "sqlite" ***
	db, err = sql.Open("sqlite", filepath) 
	if err != nil {
		return nil, fmt.Errorf("無法開啟資料庫: %w", err)
	}

	if err = db.Ping(); err != nil {
		return nil, fmt.Errorf("無法連接到資料庫: %w", err)
	}

	// 創建 tasks 資料表 (如果不存在)
	// 欄位名稱使用小寫以匹配 JSON 標籤
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS tasks (
		"id" INTEGER PRIMARY KEY,
		"name" TEXT,
		"start" TEXT,
		"durationDays" INTEGER,
		"color" TEXT,
		"priority" INTEGER
	);`

	if _, err = db.Exec(createTableSQL); err != nil {
		return nil, fmt.Errorf("無法創建資料表: %w", err)
	}

	// 檢查資料表是否為空
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM tasks").Scan(&count)
	if err != nil {
		return nil, fmt.Errorf("無法查詢任務數量: %w", err)
	}

	// 如果為空，插入預設資料
	if count == 0 {
		log.Println("資料庫為空，正在插入預設任務...")
		defaultTasks := getInitialTasks()
		// 我們將使用 saveTasksToDB 來插入，它會自動處理事務
		if err := saveTasksToDB(defaultTasks); err != nil {
			return nil, fmt.Errorf("插入預設任務失敗: %w", err)
		}
		log.Println("預設任務已成功插入。")
	}

	return db, nil
}

// loadTasksFromDB 從資料庫讀取所有任務 (此函數無需變動)
func loadTasksFromDB() ([]Task, error) {
	tasks := []Task{}
	// 根據前端邏輯，按優先度排序
	rows, err := db.Query("SELECT id, name, start, durationDays, color, priority FROM tasks ORDER BY priority")
	if err != nil {
		return nil, fmt.Errorf("查詢資料庫失敗: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var t Task
		if err := rows.Scan(&t.ID, &t.Name, &t.Start, &t.DurationDays, &t.Color, &t.Priority); err != nil {
			return nil, fmt.Errorf("掃描資料行失敗: %w", err)
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

// saveTasksToDB 將前端傳來的完整任務列表寫入資料庫 (此函數無需變動)
func saveTasksToDB(tasks []Task) error {
	// 開始事務
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("開啟事務失敗: %w", err)
	}

	// 1. 刪除所有現有任務
	if _, err := tx.Exec("DELETE FROM tasks"); err != nil {
		tx.Rollback()
		return fmt.Errorf("刪除舊任務失敗: %w", err)
	}

	// 2. 準備插入新任務
	stmt, err := tx.Prepare("INSERT INTO tasks (id, name, start, durationDays, color, priority) VALUES (?, ?, ?, ?, ?, ?)")
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("準備插入語句失敗: %w", err)
	}
	defer stmt.Close()

	// 3. 循環插入所有任務
	for _, task := range tasks {
		if _, err := stmt.Exec(task.ID, task.Name, task.Start, task.DurationDays, task.Color, task.Priority); err != nil {
			tx.Rollback()
			return fmt.Errorf("插入任務 ID %d 失敗: %w", task.ID, err)
		}
	}

	// 4. 提交事務
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("提交事務失敗: %w", err)
	}

	return nil
}

// getInitialTasks 預設任務數據 (此函數無需變動)
func getInitialTasks() []Task {
	// 為了使用方便，將起始年設定為當前年份 + 1
	nextYear := time.Now().Year() + 1
	start := time.Date(nextYear, time.January, 1, 0, 0, 0, 0, time.UTC)

	// 格式化日期為 YYYY-MM-DD
	formatDate := func(t time.Time) string {
		return t.Format("2006-01-02")
	}

	// P1: #EF4444 (Red), P2: #F97316 (Orange), P3: #FBBF24 (Amber), P4: #3B82F6 (Blue), P5: #10B981 (Green)
	return []Task{
		{ID: 1, Name: "需求收集", Start: formatDate(start), DurationDays: 7, Color: "#3B82F6", Priority: 4},
		{ID: 2, Name: "系統設計", Start: formatDate(start.AddDate(0, 0, 7)), DurationDays: 5, Color: "#F97316", Priority: 2},
		{ID: 3, Name: "後端開發", Start: formatDate(start.AddDate(0, 0, 12)), DurationDays: 12, Color: "#EF4444", Priority: 1},
		{ID: 4, Name: "前端開發", Start: formatDate(start.AddDate(0, 0, 12)), DurationDays: 10, Color: "#FBBF24", Priority: 3},
		{ID: 5, Name: "整合測試", Start: formatDate(start.AddDate(0, 0, 24)), DurationDays: 8, Color: "#10B981", Priority: 5},
	}
}

// --- 3. HTTP 處理函數 ---

// indexHandler 服務前端 HTML (此函數無需變動)
func indexHandler(w http.ResponseWriter, r *http.Request) {
	htmlBytes, err := os.ReadFile("index.html")
	if err != nil {
		http.Error(w, fmt.Sprintf("Error: Cannot find or read index.html. Detail: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(htmlBytes)
}

// apiTasksHandler 處理 GET 和 POST (此函數無需變動)
func apiTasksHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case "GET":
		tasks, err := loadTasksFromDB()
		if err != nil {
			log.Printf("Error loading tasks from DB: %v\n", err)
			http.Error(w, fmt.Sprintf(`{"error": "讀取任務數據失敗: %v"}`, err), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(tasks)

	case "POST":
		var tasks []Task
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, `{"error": "讀取請求主體失敗"}`, http.StatusBadRequest)
			return
		}

		// 解析前端傳來的小寫 JSON
		if err := json.Unmarshal(body, &tasks); err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "解析 JSON 失敗: %v"}`, err), http.StatusBadRequest)
			return
		}

		// 將新任務列表存入資料庫
		if err := saveTasksToDB(tasks); err != nil {
			log.Printf("Error saving tasks to DB: %v\n", err)
			http.Error(w, fmt.Sprintf(`{"error": "寫入任務數據失敗: %v"}`, err), http.StatusInternalServerError)
			return
		}

		fmt.Println("Info: 任務數據已成功更新並寫入資料庫。")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message": "任務數據儲存成功"}`))

	default:
		// 僅允許 GET 和 POST 方法
		http.Error(w, `{"error": "不支援的方法"}`, http.StatusMethodNotAllowed)
	}
}

// --- 4. 主函數 (啟動伺服器) ---

func main() {
	var err error
	// 初始化資料庫
	db, err = initDB("./gantt.db")
	if err != nil {
		log.Fatalf("資料庫初始化失敗: %v", err)
	}
	defer db.Close()

	// 設置路由
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/api/tasks", apiTasksHandler)

	port := "8080"
	fmt.Printf("Go 甘特圖後端已啟動 (使用 Pure Go SQLite)，請在瀏覽器中開啟 http://localhost:%s\n", port)

	// 啟動伺服器
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("伺服器啟動失敗: %v", err)
	}
}