package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
)

// 安裝: go get github.com/DATA-DOG/go-sqlmock
// 安裝: go get github.com/stretchr/testify

// --- 測試工具函數 (用於準備 Task 數據) ---

// getTestTasks 準備一組用於測試的任務數據
func getTestTasks() []Task {
	// 使用一個固定的日期，確保測試的可重複性
	testDate := time.Date(2026, time.February, 1, 0, 0, 0, 0, time.UTC)
	formatDate := func(t time.Time) string { return t.Format("2006-01-02") }

	return []Task{
		{ID: 101, Name: "測試任務 A", Start: formatDate(testDate), DurationDays: 3, Color: "#AAAAAA", Priority: 5},
		{ID: 102, Name: "測試任務 B", Start: formatDate(testDate.AddDate(0, 0, 3)), DurationDays: 2, Color: "#BBBBBB", Priority: 1},
	}
}

// --- 測試 loadTasksFromDB 函數 ---

func TestLoadTasksFromDB(t *testing.T) {
	// 1. 創建 Mock 資料庫連接和 Mock 實例
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("創建 mock 資料庫時發生錯誤: %v", err)
	}
	defer mockDB.Close()

	// 覆寫全局 db 變數為 Mock 實例，以便 loadTasksFromDB 函數使用
	db = mockDB 

	// 準備預期的測試數據
	expectedTasks := getTestTasks()

	// 2. 定義 Mock 預期行為 (當執行 SELECT 查詢時)
	rows := sqlmock.NewRows([]string{"id", "name", "start", "durationDays", "color", "priority"}).
		AddRow(expectedTasks[0].ID, expectedTasks[0].Name, expectedTasks[0].Start, expectedTasks[0].DurationDays, expectedTasks[0].Color, expectedTasks[0].Priority).
		AddRow(expectedTasks[1].ID, expectedTasks[1].Name, expectedTasks[1].Start, expectedTasks[1].DurationDays, expectedTasks[1].Color, expectedTasks[1].Priority)

	// 期望 loadTasksFromDB 會執行這個 SELECT 語句
	mock.ExpectQuery("SELECT id, name, start, durationDays, color, priority FROM tasks").
		WillReturnRows(rows)

	// 3. 執行要測試的函數
	tasks, err := loadTasksFromDB()

	// 4. 驗證結果
	assert.NoError(t, err, "loadTasksFromDB 不應返回錯誤")
	assert.Equal(t, expectedTasks, tasks, "讀取的任務數據應與預期數據匹配")

	// 5. 驗證 Mock 期望是否都滿足
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("未滿足的 Mock 期望: %s", err)
	}
}

// --- 測試 saveTasksToDB 函數 ---

func TestSaveTasksToDB(t *testing.T) {
	// 1. 創建 Mock 資料庫連接和 Mock 實例
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("創建 mock 資料庫時發生錯誤: %v", err)
	}
	defer mockDB.Close()

	// 覆寫全局 db 變數為 Mock 實例
	db = mockDB 

	// 準備要儲存的數據
	tasksToSave := getTestTasks()
    
	// 2. 定義 Mock 預期行為

	// 期望開啟事務
	mock.ExpectBegin()

	// 期望刪除舊數據 (DELETE FROM tasks)
	mock.ExpectExec("DELETE FROM tasks").
		WillReturnResult(sqlmock.NewResult(0, 0))

    // *** 關鍵修正：預期 Prepare 呼叫 ***
    // 我們需要預期 prepare 語句的完整 SQL。
    // 這個 ExpectPrepare 會模擬成功地創建了一個 *sql.Stmt
    prep := mock.ExpectPrepare("INSERT INTO tasks \\(id, name, start, durationDays, color, priority\\) VALUES \\(\\$1, \\$2, \\$3, \\$4, \\$5, \\$6\\)")

    // 期望執行第一個任務的 Exec (現在是透過 prep.ExpectExec 來模擬對已 Prepare 的語句的執行)
	prep.ExpectExec().
		WithArgs(tasksToSave[0].ID, tasksToSave[0].Name, tasksToSave[0].Start, tasksToSave[0].DurationDays, tasksToSave[0].Color, tasksToSave[0].Priority).
		WillReturnResult(sqlmock.NewResult(1, 1))

    // 期望執行第二個任務的 Exec
	prep.ExpectExec().
		WithArgs(tasksToSave[1].ID, tasksToSave[1].Name, tasksToSave[1].Start, tasksToSave[1].DurationDays, tasksToSave[1].Color, tasksToSave[1].Priority).
		WillReturnResult(sqlmock.NewResult(2, 1))

	// 期望提交事務
	mock.ExpectCommit()

	// 3. 執行要測試的函數
	err = saveTasksToDB(tasksToSave)

	// 4. 驗證結果
	assert.NoError(t, err, "saveTasksToDB 不應返回錯誤")

	// 5. 驗證 Mock 期望是否都滿足
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("未滿足的 Mock 期望: %s", err)
	}
}

// --- 測試錯誤情況 (可選，但推薦) ---

func TestLoadTasksFromDB_QueryError(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("創建 mock 資料庫時發生錯誤: %v", err)
	}
	defer mockDB.Close()
	db = mockDB

	// 期望查詢失敗並返回一個錯誤
	mock.ExpectQuery("SELECT id, name, start, durationDays, color, priority FROM tasks").
		WillReturnError(fmt.Errorf("模擬查詢錯誤"))

	tasks, err := loadTasksFromDB()
	
	// 期望返回錯誤
	assert.Error(t, err, "loadTasksFromDB 應返回錯誤")
	assert.Nil(t, tasks, "tasks 應為 nil")
	
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("未滿足的 Mock 期望: %s", err)
	}
}