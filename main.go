package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"
)

// --- 1. 後端數據結構 (與前端任務結構匹配 - 全部小寫) ---

// Task 結構體定義了甘特圖中單一任務的數據。
// 注意: 結構體成員名稱必須是大寫開頭才能被 JSON 序列化/反序列化。
// JSON tag 使用小寫以匹配前端 JS 物件屬性名稱。
type Task struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Start        string `json:"start"` // YYYY-MM-DD 格式
	DurationDays int    `json:"durationDays"`
	Color        string `json:"color"`
	Priority     int    `json:"priority"`
}

// 數據檔案路徑
const jsonFilePath = "gantt.json"

// Mutex 用於確保對 gantt.json 文件的讀寫是線程安全的。
var dataMutex sync.Mutex

// --- 2. 檔案操作與數據加載 ---

// loadTasksFromFile 嘗試從 gantt.json 讀取任務列表。
func loadTasksFromFile() ([]Task, error) {
	// 嘗試讀取檔案
	data, err := os.ReadFile(jsonFilePath)
	if os.IsNotExist(err) {
		// 如果文件不存在，創建一個包含預設任務的檔案
		fmt.Printf("Info: %s 不存在，正在創建預設檔案。\n", jsonFilePath)
		tasks := getInitialTasks()

		// 在調用 saveTasksToFile 之前，必須確保沒有持有鎖定。
		if err := saveTasksToFile(tasks); err != nil {
			return nil, fmt.Errorf("無法創建並寫入預設檔案: %w", err)
		}
		return tasks, nil
	} else if err != nil {
		return nil, fmt.Errorf("讀取檔案失敗: %w", err)
	}

	// 確保在解析 JSON 時進行鎖定，以防止其他協程同時寫入
	dataMutex.Lock()
	defer dataMutex.Unlock()

	var tasks []Task
	if err := json.Unmarshal(data, &tasks); err != nil {
		return nil, fmt.Errorf("解析 JSON 失敗: %w", err)
	}

	return tasks, nil
}

// saveTasksToFile 將任務列表寫入 gantt.json。
func saveTasksToFile(tasks []Task) error {
	dataMutex.Lock()
	defer dataMutex.Unlock()

	data, err := json.MarshalIndent(tasks, "", "  ")
	if err != nil {
		return fmt.Errorf("序列化 JSON 失敗: %w", err)
	}

	// 寫入檔案
	if err := os.WriteFile(jsonFilePath, data, 0644); err != nil {
		return fmt.Errorf("寫入檔案失敗: %w", err)
	}
	return nil
}

// 預設任務數據 (如果 gantt.json 不存在)
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
		{ID: 1, Name: "需求收集", Start: formatDate(start), DurationDays: 7, Color: "#3B82F6", Priority: 4},                    // P4 - 低 (藍色)
		{ID: 2, Name: "系統設計", Start: formatDate(start.AddDate(0, 0, 7)), DurationDays: 5, Color: "#F97316", Priority: 2},   // P2 - 高 (橘色)
		{ID: 3, Name: "後端開發", Start: formatDate(start.AddDate(0, 0, 12)), DurationDays: 12, Color: "#EF4444", Priority: 1}, // P1 - 緊急 (紅色)
		{ID: 4, Name: "前端開發", Start: formatDate(start.AddDate(0, 0, 12)), DurationDays: 10, Color: "#FBBF24", Priority: 3}, // P3 - 中 (黃色)
		{ID: 5, Name: "整合測試", Start: formatDate(start.AddDate(0, 0, 24)), DurationDays: 8, Color: "#10B981", Priority: 5},  // P5 - 最低 (綠色)
	}
}

// --- 3. HTTP 處理函數 ---

// indexHandler 服務前端 HTML/JS 程式碼。現在從檔案系統讀取 index.html。
func indexHandler(w http.ResponseWriter, r *http.Request) {
	// 讀取 index.html 檔案
	htmlBytes, err := os.ReadFile("index.html")
	if err != nil {
		// 發生錯誤時顯示一個更明確的訊息
		http.Error(w, fmt.Sprintf("Error: Cannot find or read index.html in the current directory. Please make sure both main.go and index.html are present. Detail: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write(htmlBytes)
}

// apiTasksHandler 處理 GET (讀取), POST (寫入) 和 DELETE (刪除) 任務數據。
func apiTasksHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	switch r.Method {
	case "GET":
		tasks, err := loadTasksFromFile()
		if err != nil {
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

		if err := json.Unmarshal(body, &tasks); err != nil {
			http.Error(w, `{"error": "解析 JSON 失敗"}`, http.StatusBadRequest)
			return
		}

		if err := saveTasksToFile(tasks); err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "寫入任務數據失敗: %v"}`, err), http.StatusInternalServerError)
			return
		}

		fmt.Printf("Info: 任務數據已成功更新並寫入 %s\n", jsonFilePath)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"message": "任務數據儲存成功"}`))

	case "DELETE":
		// 處理任務刪除
		queryID := r.URL.Query().Get("id")
		if queryID == "" {
			http.Error(w, `{"error": "Missing task ID"}`, http.StatusBadRequest)
			return
		}

		taskID, err := strconv.Atoi(queryID)
		if err != nil {
			http.Error(w, `{"error": "Invalid task ID format"}`, http.StatusBadRequest)
			return
		}

		// 1. 載入目前任務
		currentTasks, err := loadTasksFromFile()
		if err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "Failed to load tasks for deletion: %v"}`, err), http.StatusInternalServerError)
			return
		}

		// 2. 查找並移除任務
		found := false
		newTasks := []Task{}
		for _, task := range currentTasks {
			if task.ID != taskID {
				newTasks = append(newTasks, task)
			} else {
				found = true
			}
		}

		if !found {
			http.Error(w, fmt.Sprintf(`{"error": "Task with ID %d not found"}`, taskID), http.StatusNotFound)
			return
		}

		// 3. 儲存更新後的列表
		if err := saveTasksToFile(newTasks); err != nil {
			http.Error(w, fmt.Sprintf(`{"error": "Failed to save tasks after deletion: %v"}`, err), http.StatusInternalServerError)
			return
		}

		fmt.Printf("Info: Task ID %d deleted and data saved to %s\n", taskID, jsonFilePath)
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(fmt.Sprintf(`{"message": "Task ID %d deleted successfully"}`, taskID)))

	default:
		// 僅允許 GET, POST 和 DELETE 方法
		http.Error(w, `{"error": "不支援的方法"}`, http.StatusMethodNotAllowed)
	}
}

// --- 4. 主函數 (啟動伺服器) ---

func main() {
	// 設置路由
	http.HandleFunc("/", indexHandler)
	http.HandleFunc("/api/tasks", apiTasksHandler)

	port := "8080"
	fmt.Printf("Go 甘特圖後端已啟動，請在瀏覽器中開啟 http://localhost:%s\n", port)

	// 首次嘗試載入數據，確保 gantt.json 存在或被創建
	// 注意：這裡移除了原有的 defer unlock 邏輯以避免死鎖
	if _, err := loadTasksFromFile(); err != nil {
		fmt.Printf("Error: 初始載入/創建檔案失敗: %v\n", err)
	}

	// 啟動伺服器
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		fmt.Printf("伺服器啟動失敗: %v\n", err)
	}
}
