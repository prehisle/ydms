-- 本地开发数据库初始化脚本
-- 自动创建 ndr 和 ydms 数据库

-- 创建 ndr 数据库
CREATE DATABASE ndr;

-- 创建 ydms 数据库
CREATE DATABASE ydms;

-- 启用 ltree 扩展（ndr 需要）
\c ndr
CREATE EXTENSION IF NOT EXISTS ltree;
CREATE EXTENSION IF NOT EXISTS pg_trgm;

\c ydms
CREATE EXTENSION IF NOT EXISTS ltree;
