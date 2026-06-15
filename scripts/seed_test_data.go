//go:build ignore

package main

import (
	"fmt"
	"log"

	"edu_market/model"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

func main() {
	dsn := "root:123456@tcp(127.0.0.1:3306)/edu_market?charset=utf8mb4&parseTime=True&loc=Local"
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("数据库连接失败: %v", err)
	}

	user := &model.User{Username: "seed_test_user", Role: "user", PasswordHash: "$2a$10$placeholder"}
	db.Where("username = ?", "seed_test_user").FirstOrCreate(user)
	fmt.Printf("User: id=%d\n", user.ID)

	cats := map[string]uint{}
	for _, name := range []string{"编程开发", "设计创意", "语言学习"} {
		var cat model.Category
		db.Where("name = ?", name).FirstOrCreate(&cat, &model.Category{Name: name})
		cats[name] = cat.ID
	}

	// ===== Python 入门 =====
	m1 := create(db, user.ID, cats["编程开发"], "Python 从入门到实战", 19.90,
		"适合零基础的 Python 教程，涵盖基础语法、面向对象、Web 开发、数据分析四大模块。45 个代码示例，12 个实战项目。")
	addDoc(db, m1.ID, "第一章 Python 基础",
		`# 第一章 Python 基础

## 1.1 环境搭建

Python 可以从 python.org 下载，推荐 Python 3.10 以上版本。安装后在终端输入 python --version 验证。

## 1.2 第一个程序

print("Hello, World!") 是最简单的 Python 程序。print 是内置函数，用于输出内容到控制台。

## 1.3 变量与数据类型

Python 是动态类型语言，变量不需要声明类型。常用类型包括 int（整数）、float（浮点数）、str（字符串）、bool（布尔值）、list（列表）和 dict（字典）。

## 1.4 条件判断

Python 使用 if/elif/else 做条件判断。score = 85; if score >= 90: print("优秀") elif score >= 80: print("良好") else: print("继续努力")。缩进是 Python 的语法要求，一般用 4 个空格。`)
	addDoc(db, m1.ID, "第二章 函数与模块",
		`# 第二章 函数与模块

## 2.1 定义函数

def greet(name): return f"你好，{name}！" Python 使用 def 关键字定义函数，函数名用小写加下划线。docstring 用三重引号写在函数体第一行。

## 2.2 参数类型

Python 支持位置参数、关键字参数、默认参数和可变参数（*args 和 **kwargs）。默认参数值在函数定义时计算一次，可变对象作默认值有陷阱。

## 2.3 模块导入

import math 导入整个模块，from datetime import datetime 导入特定函数，from my_utils import helper 导入自定义模块。

## 2.4 Lambda 函数

lambda 是匿名函数，适合简单操作。students.sort(key=lambda x: x[1], reverse=True) 按成绩降序排序。`)
	addDoc(db, m1.ID, "第三章 面向对象编程",
		`# 第三章 面向对象编程

## 3.1 类与对象

面向对象编程是 Python 的核心特性。class Student: def __init__(self, name, age): 初始化属性。self 代表实例本身，是方法的第一个参数。

## 3.2 继承

子类继承父类的属性和方法。class GraduateStudent(Student): def __init__(self, name, age, thesis): super().__init__(name, age) 调用父类构造。

## 3.3 魔法方法

__init__ 构造，__add__ 加法，__str__ 字符串表示。class Vector: def __add__(self, other): return Vector(self.x + other.x, self.y + other.y)。

## 3.4 封装与属性装饰器

@property 装饰器把方法变成属性访问。class BankAccount: @property def balance(self): return self._balance。存款用 deposit 方法而非直接赋值。`)

	// ===== UI 设计 =====
	m2 := create(db, user.ID, cats["设计创意"], "UI 设计速成：从零到入职", 24.90,
		"完整的 UI 设计入门课程。从设计思维到 Figma 实操，7 个真实项目帮你做出可以放进作品集的设计稿。")

	addDoc(db, m2.ID, "第一章 设计思维",
		`# 第一章 设计思维

## 1.1 设计不是画画

设计是解决问题。好的设计 = 好用 + 好看。尼尔森十大可用性原则：系统状态可见性、系统与真实世界匹配、用户控制与自由、一致性与标准、错误预防、识别而非回忆、灵活高效、简洁美观、帮助识别诊断错误、帮助文档。

## 1.2 用户研究

用户画像（Persona）帮助团队聚焦目标用户。包含基本信息（年龄、职业）、使用场景、痛点和目标。设计前花 30 分钟做用户研究能节省数小时的返工。

## 1.3 信息架构

信息架构（IA）决定用户找到信息的难易程度。卡片分类法是构建 IA 的常用方法——把功能写在卡片上让用户分组，发现用户的心智模型。`)
	addDoc(db, m2.ID, "第二章 Figma 实战",
		`# 第二章 Figma 实战

## 2.1 界面速览

Figma 是云端协作设计工具。核心功能：Frame（画板）、Auto Layout（自动布局类似 Flexbox）、Components（可复用组件）、Variants（组件变体，如按钮的不同状态）。

## 2.2 制作按钮组件

创建 Frame 命名 Button，设置 Auto Layout 水平排列间距 8px，添加圆角 8px，创建 Text Layer，应用颜色样式。主按钮用品牌色，次按钮用灰色描边，危险按钮用红色。

## 2.3 设计系统基础

设计系统 = 可复用的设计组件 + 规范。颜色规范推荐用 HSL 色彩模型：H 色相、S 饱和度、L 亮度。生成色阶时保持 H 和 S 不变，只调整 L。`)

	// ===== 英语通关 =====
	m3 := create(db, user.ID, cats["语言学习"], "英语零基础通关指南", 9.90,
		"专为英语初学者设计。从 26 个字母到日常对话，每天 20 分钟，30 天开口说英语。包含 300+ 核心词汇、50 个高频句型、20 个生活场景。")

	addDoc(db, m3.ID, "第一课 字母与发音",
		`# 第一课 字母与发音

## 26 个字母

英语有 26 个字母，分元音（a, e, i, o, u）和辅音（其余 21 个）。字母名称和发音不同，这是初学者最容易混淆的地方。

## 元音发音规则

A 在开音节读 /ei/（cake），闭音节读 /ae/（cat）。E 在开音节读 /i:/（he），闭音节读 /e/（bed）。I 在开音节读 /ai/（bike），闭音节读 /i/（big）。

## 自然拼读法

不需要看音标就能读出单词。学习 44 个音素和拼写规则。80% 的英语单词符合拼读规则，剩下 20% 需要单独记忆。`)
	addDoc(db, m3.ID, "第二课 高频句型 50 句",
		`# 第二课 高频句型 50 句

## 问候

Hello, how are you? / I'm fine, thank you. / Nice to meet you. / Good morning/afternoon/evening.

## 介绍自己

My name is [Name]. / I'm from [Country]. / I'm [age] years old. / I work as a [job title].

## 日常对话

How much is this? / Where is the restroom? / Can I have the menu? / I'd like to order... / Thank you very much. / You're welcome. / Excuse me. / I'm sorry. / No problem.`)

	fmt.Println("=== DONE ===")
	fmt.Printf("Materials: Python(id=%d) UI(id=%d) English(id=%d)\n", m1.ID, m2.ID, m3.ID)
	fmt.Println("Start server and test RAG: ask Agent 'Python讲什么' or '英语有多少字母'")
}

func create(db *gorm.DB, uid, cid uint, t string, p float64, d string) *model.Material {
	m := &model.Material{Title: t, Description: d, Price: p, CategoryID: cid, UserID: uid, Status: "published"}
	db.Create(m)
	fmt.Printf("Material: %s (id=%d)\n", t, m.ID)
	return m
}

func addDoc(db *gorm.DB, mid uint, title, content string) {
	doc := &model.Document{MaterialID: mid, Title: title, Content: content, IsFreePreview: false, Status: "published"}
	db.Create(doc)
	fmt.Printf("  Doc: %s (id=%d)\n", title, doc.ID)
}
