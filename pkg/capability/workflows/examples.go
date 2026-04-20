package workflow

import (
	"fmt"
	"time"
)

// ExampleCrossAppWorkflow 创建跨应用工作流示例
// 这个示例展示了一个完整的数据处理流程：
// 1. 从文件中读取数据
// 2. 在浏览器中搜索信息
// 3. 处理数据
// 4. 保存结果到文档
// 5. 发送结果给联系人
func ExampleCrossAppWorkflow() *Graph {
	graph := NewGraph(
		"数据处理与分享工作流",
		"从文件读取数据，搜索信息，处理后保存并分享的完整工作流",
	)

	graph.Version = "1.0.0"
	graph.Author = "AnyClaw Team"
	graph.Tags = []string{"data-processing", "cross-app", "automation"}

	// 输入参数
	graph.AddInputParam("input_file", "string", "输入文件路径", true, nil)
	graph.AddInputParam("search_query", "string", "搜索查询", true, nil)
	graph.AddInputParam("output_file", "string", "输出文件路径", false, "result.txt")
	graph.AddInputParam("recipient", "string", "收件人", false, "")

	// 变量
	graph.AddVariable("file_content", "string", "文件内容", "")
	graph.AddVariable("search_results", "array", "搜索结果", []any{})
	graph.AddVariable("processed_data", "object", "处理后的数据", map[string]any{})
	graph.AddVariable("document_path", "string", "文档路径", "")

	// 输出参数
	graph.AddOutputParam("success", "boolean", "执行是否成功", "send_result.success")
	graph.AddOutputParam("document_url", "string", "生成的文档路径", "$document_path")
	graph.AddOutputParam("recipient_notified", "boolean", "收件人是否已通知", "send_result.recipient_notified")

	// 节点1: 读取文件
	readFileID := graph.AddActionNode(
		"读取输入文件",
		"使用文件管理器读取输入文件内容",
		"file-manager",
		"read-file",
		map[string]any{
			"path": "$input_file",
		},
	)

	// 节点2: 解析数据
	parseDataID := graph.AddActionNode(
		"解析文件数据",
		"解析文件内容为结构化数据",
		"data-processor",
		"parse-data",
		map[string]any{
			"content": "$read_file.content",
			"format":  "auto",
		},
	)

	// 节点3: 浏览器搜索
	searchWebID := graph.AddActionNode(
		"网页搜索",
		"在浏览器中搜索相关信息",
		"browser-local",
		"search-web",
		map[string]any{
			"query": "$search_query",
			"count": 5,
		},
	)

	// 节点4: 处理数据
	processDataID := graph.AddActionNode(
		"处理数据",
		"结合文件数据和搜索结果进行处理",
		"data-processor",
		"process-data",
		map[string]any{
			"file_data":   "$parse_data.parsed",
			"web_results": "$search_web.results",
			"operation":   "enrich",
		},
	)

	// 节点5: 保存到文档
	saveDocumentID := graph.AddActionNode(
		"保存到文档",
		"将处理结果保存到文档",
		"office-local",
		"save-document",
		map[string]any{
			"data":        "$process_data.result",
			"output_path": "$output_file",
			"format":      "markdown",
		},
	)

	// 条件节点: 检查是否需要发送
	checkSendID := graph.AddConditionNode(
		"检查是否需要发送",
		"检查是否有收件人，决定是否需要发送",
		"$recipient != ''",
	)

	// 节点6: 发送结果
	sendResultID := graph.AddActionNode(
		"发送结果",
		"通过消息应用发送结果给收件人",
		"qq-local",
		"send-message",
		map[string]any{
			"contact":    "$recipient",
			"message":    "数据处理已完成，结果已保存到：$save_document.path",
			"attachment": "$save_document.path",
		},
	)

	// 节点7: 记录日志
	logResultID := graph.AddActionNode(
		"记录执行日志",
		"记录工作流执行结果",
		"system-logger",
		"log-event",
		map[string]any{
			"event": "workflow_completed",
			"data": map[string]any{
				"workflow":    graph.Name,
				"success":     true,
				"output_file": "$save_document.path",
				"recipient":   "$recipient",
			},
		},
	)

	// 边连接
	graph.AddEdge(readFileID, parseDataID, "default")
	graph.AddEdge(parseDataID, searchWebID, "default")
	graph.AddEdge(searchWebID, processDataID, "default")
	graph.AddEdge(processDataID, saveDocumentID, "default")
	graph.AddEdge(saveDocumentID, checkSendID, "default")

	// 条件边
	graph.AddEdge(checkSendID, sendResultID, "condition_true")
	graph.AddEdge(checkSendID, logResultID, "condition_false")
	graph.AddEdge(sendResultID, logResultID, "default")

	// 设置节点位置（用于可视化）
	setNodePositions(graph)

	return graph
}

// ExampleSimpleFileProcessing 创建简单的文件处理工作流
func ExampleSimpleFileProcessing() *Graph {
	graph := NewGraph(
		"简单文件处理工作流",
		"读取文件、处理内容、保存结果的简单工作流",
	)

	graph.AddInputParam("source_file", "string", "源文件路径", true, nil)
	graph.AddInputParam("target_file", "string", "目标文件路径", true, nil)
	graph.AddInputParam("operation", "string", "处理操作", false, "copy")

	// 节点1: 检查源文件
	checkFileID := graph.AddActionNode(
		"检查源文件",
		"检查源文件是否存在且可读",
		"file-manager",
		"check-file",
		map[string]any{
			"path":  "$source_file",
			"check": "exists_readable",
		},
	)

	// 条件节点: 根据操作类型选择处理方式
	checkOperationID := graph.AddConditionNode(
		"检查操作类型",
		"根据操作类型决定处理方式",
		"$operation == 'copy'",
	)

	// 节点2: 复制文件
	copyFileID := graph.AddActionNode(
		"复制文件",
		"将源文件复制到目标位置",
		"file-manager",
		"copy-file",
		map[string]any{
			"source":      "$source_file",
			"destination": "$target_file",
		},
	)

	// 节点3: 移动文件
	moveFileID := graph.AddActionNode(
		"移动文件",
		"将源文件移动到目标位置",
		"file-manager",
		"move-file",
		map[string]any{
			"source":      "$source_file",
			"destination": "$target_file",
		},
	)

	// 节点4: 验证结果
	verifyResultID := graph.AddActionNode(
		"验证结果",
		"验证目标文件是否正确创建",
		"file-manager",
		"verify-file",
		map[string]any{
			"path":          "$target_file",
			"expected_size": "$check_file.size",
			"expected_hash": "$check_file.hash",
		},
	)

	// 连接边
	graph.AddEdge(checkFileID, checkOperationID, "default")
	graph.AddEdge(checkOperationID, copyFileID, "condition_true")
	graph.AddEdge(checkOperationID, moveFileID, "condition_false")
	graph.AddEdge(copyFileID, verifyResultID, "default")
	graph.AddEdge(moveFileID, verifyResultID, "default")

	setNodePositions(graph)

	return graph
}

// ExampleEmailWorkflow 创建邮件处理工作流
func ExampleEmailWorkflow() *Graph {
	graph := NewGraph(
		"邮件处理工作流",
		"检查邮件、处理附件、回复邮件的完整工作流",
	)

	graph.AddInputParam("email_account", "string", "邮箱账户", true, nil)
	graph.AddInputParam("subject_filter", "string", "主题过滤器", false, "")
	graph.AddInputParam("attachment_dir", "string", "附件保存目录", false, "./attachments")

	// 节点1: 检查新邮件
	checkEmailID := graph.AddActionNode(
		"检查新邮件",
		"检查指定账户的新邮件",
		"email-client",
		"check-emails",
		map[string]any{
			"account":     "$email_account",
			"unread_only": true,
			"max_count":   10,
		},
	)

	// 条件节点: 是否有新邮件
	hasNewEmailsID := graph.AddConditionNode(
		"检查是否有新邮件",
		"检查是否有满足条件的新邮件",
		"len($check_email.emails) > 0",
	)

	// 循环节点: 处理每封邮件
	processEmailsID := graph.AddActionNode(
		"处理邮件",
		"处理每封符合条件的邮件",
		"email-processor",
		"process-email",
		map[string]any{
			"email":          "$current_email",
			"attachment_dir": "$attachment_dir",
		},
	)

	// 节点2: 发送确认回复
	sendReplyID := graph.AddActionNode(
		"发送确认回复",
		"发送自动确认回复",
		"email-client",
		"send-reply",
		map[string]any{
			"email":    "$process_email.processed",
			"template": "auto_reply",
		},
	)

	// 节点3: 记录处理结果
	logEmailID := graph.AddActionNode(
		"记录邮件处理",
		"记录邮件处理结果",
		"system-logger",
		"log-email",
		map[string]any{
			"email_id":    "$process_email.email_id",
			"status":      "$process_email.status",
			"attachments": "$process_email.attachments",
		},
	)

	// 连接边
	graph.AddEdge(checkEmailID, hasNewEmailsID, "default")
	graph.AddEdge(hasNewEmailsID, processEmailsID, "condition_true")
	graph.AddEdge(processEmailsID, sendReplyID, "default")
	graph.AddEdge(sendReplyID, logEmailID, "default")

	setNodePositions(graph)

	return graph
}

// setNodePositions 设置节点位置（用于可视化）
func setNodePositions(graph *Graph) {
	// 简单的位置布局
	positions := []Position{
		{X: 100, Y: 100}, // 开始节点
		{X: 300, Y: 100}, // 处理节点1
		{X: 500, Y: 100}, // 处理节点2
		{X: 300, Y: 300}, // 条件节点
		{X: 500, Y: 300}, // 分支1
		{X: 700, Y: 300}, // 分支2
		{X: 400, Y: 500}, // 结束节点
	}

	for i := range graph.Nodes {
		if i < len(positions) {
			graph.Nodes[i].Position = positions[i]
		}
	}
}

// ExportExampleGraphs 导出示例工作流图
func ExportExampleGraphs() map[string]*Graph {
	graphs := make(map[string]*Graph)

	graphs["cross-app-data-processing"] = ExampleCrossAppWorkflow()
	graphs["simple-file-processing"] = ExampleSimpleFileProcessing()
	graphs["email-processing"] = ExampleEmailWorkflow()

	return graphs
}

// CreateGraphFromTemplate 从模板创建工作流图
func CreateGraphFromTemplate(templateName string, params map[string]any) (*Graph, error) {
	templates := ExportExampleGraphs()

	if template, ok := templates[templateName]; ok {
		// 克隆图
		graph := *template

		// 应用参数
		for key, value := range params {
			// 更新输入参数的默认值
			for i := range graph.Inputs {
				if graph.Inputs[i].Name == key {
					graph.Inputs[i].Default = value
				}
			}

			// 更新变量初始值
			for i := range graph.Variables {
				if graph.Variables[i].Name == key {
					graph.Variables[i].InitialValue = value
				}
			}
		}

		graph.ID = generateGraphID()
		graph.CreatedAt = time.Now().UTC()
		graph.UpdatedAt = graph.CreatedAt

		return &graph, nil
	}

	return nil, fmt.Errorf("template not found: %s", templateName)
}

// ValidateAndFixGraph 验证并修复工作流图
func ValidateAndFixGraph(graph *Graph) error {
	// 基本验证
	if err := graph.Validate(); err != nil {
		return err
	}

	// 检查节点ID唯一性
	nodeIDs := make(map[string]bool)
	for i, node := range graph.Nodes {
		if node.ID == "" {
			graph.Nodes[i].ID = generateNodeID()
		}
		if nodeIDs[graph.Nodes[i].ID] {
			return fmt.Errorf("duplicate node ID: %s", graph.Nodes[i].ID)
		}
		nodeIDs[graph.Nodes[i].ID] = true
	}

	// 检查边引用的节点是否存在
	for i, edge := range graph.Edges {
		if edge.ID == "" {
			graph.Edges[i].ID = generateEdgeID()
		}

		if !nodeIDs[edge.Source] {
			return fmt.Errorf("edge references non-existent source node: %s", edge.Source)
		}
		if !nodeIDs[edge.Target] {
			return fmt.Errorf("edge references non-existent target node: %s", edge.Target)
		}
	}

	return nil
}
