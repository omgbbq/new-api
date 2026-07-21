package controller

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/QuantumNous/new-api/common"
	"github.com/QuantumNous/new-api/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func setupLogTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	gin.SetMode(gin.TestMode)
	common.SetDatabaseTypes(common.DatabaseTypeSQLite, common.DatabaseTypeSQLite)
	common.RedisEnabled = false
	common.LogConsumeEnabled = true

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	model.DB = db
	model.LOG_DB = db

	require.NoError(t, db.AutoMigrate(&model.Log{}, &model.User{}))

	t.Cleanup(func() {
		sqlDB, err := db.DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

func TestRecordConsumeLog_StoresResponseId(t *testing.T) {
	db := setupLogTestDB(t)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/v1/chat/completions", nil)
	c.Set("id", 1)
	c.Set("username", "testuser")
	c.Set(common.RequestIdKey, "req-001")
	c.Set(common.ResponseIdKey, "chatcmpl-476855aa-3e9e-98a8-ae5b-b69170db06d3")

	model.RecordConsumeLog(c, 1, model.RecordConsumeLogParams{
		ChannelId: 1,
		ModelName: "gpt-4",
		TokenName: "test-token",
		Quota:     100,
		Content:   "test",
		TokenId:   1,
		IsStream:  false,
		Group:     "default",
	})

	var log model.Log
	err := db.First(&log).Error
	require.NoError(t, err)
	assert.Equal(t, "chatcmpl-476855aa-3e9e-98a8-ae5b-b69170db06d3", log.ResponseId)
	assert.Equal(t, "req-001", log.RequestId)
}

func TestGetAllLogs_FiltersByResponseId(t *testing.T) {
	db := setupLogTestDB(t)

	logs := []*model.Log{
		{UserId: 1, CreatedAt: 1000, Type: model.LogTypeConsume, ModelName: "gpt-4", ResponseId: "chatcmpl-aaa", RequestId: "r1"},
		{UserId: 1, CreatedAt: 1001, Type: model.LogTypeConsume, ModelName: "gpt-4", ResponseId: "chatcmpl-bbb", RequestId: "r2"},
		{UserId: 1, CreatedAt: 1002, Type: model.LogTypeConsume, ModelName: "gpt-4", ResponseId: "", RequestId: "r3"},
	}
	for _, l := range logs {
		require.NoError(t, db.Create(l).Error)
	}

	results, total, err := model.GetAllLogs(0, 0, 0, "", "", "", 0, 10, 0, "", "", "", "chatcmpl-aaa")
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, results, 1)
	assert.Equal(t, "chatcmpl-aaa", results[0].ResponseId)
}

func TestGetAllLogs_EmptyResponseIdReturnsAll(t *testing.T) {
	db := setupLogTestDB(t)

	logs := []*model.Log{
		{UserId: 1, CreatedAt: 1000, Type: model.LogTypeConsume, ModelName: "gpt-4", ResponseId: "chatcmpl-aaa", RequestId: "r1"},
		{UserId: 1, CreatedAt: 1001, Type: model.LogTypeConsume, ModelName: "gpt-4", ResponseId: "chatcmpl-bbb", RequestId: "r2"},
	}
	for _, l := range logs {
		require.NoError(t, db.Create(l).Error)
	}

	results, total, err := model.GetAllLogs(0, 0, 0, "", "", "", 0, 10, 0, "", "", "", "")
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.Len(t, results, 2)
}

func TestGetUserLogs_FiltersByResponseId(t *testing.T) {
	db := setupLogTestDB(t)

	logs := []*model.Log{
		{UserId: 42, CreatedAt: 1000, Type: model.LogTypeConsume, ModelName: "gpt-4", ResponseId: "chatcmpl-xxx", RequestId: "r1"},
		{UserId: 42, CreatedAt: 1001, Type: model.LogTypeConsume, ModelName: "gpt-4", ResponseId: "chatcmpl-yyy", RequestId: "r2"},
	}
	for _, l := range logs {
		require.NoError(t, db.Create(l).Error)
	}

	results, total, err := model.GetUserLogs(42, 0, 0, 0, "", "", 0, 10, "", "", "", "chatcmpl-xxx")
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, results, 1)
	assert.Equal(t, "chatcmpl-xxx", results[0].ResponseId)
}
