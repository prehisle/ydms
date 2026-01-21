import { type FC } from "react";
import { Card, Space, Typography, Breadcrumb, Row, Col, Statistic, Button } from "antd";
import {
  HomeOutlined,
  TeamOutlined,
  KeyOutlined,
  SettingOutlined,
  FileSearchOutlined,
  HeartOutlined,
  SyncOutlined,
} from "@ant-design/icons";
import { Link, useNavigate } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { listUsers } from "../../../api/users";
import { getAPIKeyStats } from "../../../api/apikeys";
import { useAuth } from "../../../contexts/AuthContext";

const { Title, Paragraph } = Typography;

interface SystemModuleCardProps {
  title: string;
  description: string;
  icon: React.ReactNode;
  to: string;
  disabled?: boolean;
}

function SystemModuleCard({ title, description, icon, to, disabled }: SystemModuleCardProps) {
  const navigate = useNavigate();

  return (
    <Card
      hoverable={!disabled}
      style={{ height: "100%", opacity: disabled ? 0.6 : 1 }}
      onClick={disabled ? undefined : () => navigate(to)}
    >
      <Space direction="vertical" size="small">
        <Space>
          {icon}
          <Title level={5} style={{ margin: 0 }}>
            {title}
          </Title>
        </Space>
        <Paragraph type="secondary" style={{ margin: 0 }}>
          {description}
        </Paragraph>
      </Space>
    </Card>
  );
}

/**
 * 系统管理仪表盘页面
 * 展示系统概览和各管理模块入口
 */
export const SystemDashboard: FC = () => {
  const { user } = useAuth();
  const isSuperAdmin = user?.role === "super_admin";

  // 获取用户数量
  const { data: usersData } = useQuery({
    queryKey: ["users"],
    queryFn: listUsers,
    enabled: isSuperAdmin,
  });

  // 获取 API Key 统计
  const { data: apiKeyStats } = useQuery({
    queryKey: ["apiKeyStats"],
    queryFn: getAPIKeyStats,
    enabled: isSuperAdmin,
  });

  const modules: SystemModuleCardProps[] = [
    {
      title: "用户管理",
      description: "管理系统用户，设置角色和权限",
      icon: <TeamOutlined style={{ fontSize: 24, color: "#1890ff" }} />,
      to: "/system/users",
    },
    {
      title: "API Key 管理",
      description: "创建和管理 API 访问密钥",
      icon: <KeyOutlined style={{ fontSize: 24, color: "#52c41a" }} />,
      to: "/system/api-keys",
    },
    {
      title: "工作流管理",
      description: "查看和管理 Prefect 工作流同步",
      icon: <SyncOutlined style={{ fontSize: 24, color: "#13c2c2" }} />,
      to: "/system/workflows",
    },
    {
      title: "系统配置",
      description: "配置系统参数和集成设置",
      icon: <SettingOutlined style={{ fontSize: 24, color: "#722ed1" }} />,
      to: "/system/config",
      disabled: true,
    },
    {
      title: "操作审计",
      description: "查看系统操作日志和审计记录",
      icon: <FileSearchOutlined style={{ fontSize: 24, color: "#fa8c16" }} />,
      to: "/system/audit",
      disabled: true,
    },
    {
      title: "系统状态",
      description: "查看服务健康状态和依赖连通性",
      icon: <HeartOutlined style={{ fontSize: 24, color: "#eb2f96" }} />,
      to: "/system/health",
      disabled: true,
    },
  ];

  return (
    <div style={{ padding: "24px", height: "100%", overflow: "auto" }}>
      <Space direction="vertical" size="large" style={{ width: "100%" }}>
        <Breadcrumb
          items={[
            { title: <Link to="/"><HomeOutlined /></Link> },
            { title: "系统管理" },
          ]}
        />

        <Card>
          <Title level={4}>系统管理</Title>
          <Paragraph type="secondary">
            管理系统用户、API Key、配置和审计日志
          </Paragraph>

          {/* 统计概览 */}
          <Row gutter={[16, 16]} style={{ marginTop: 24, marginBottom: 24 }}>
            <Col xs={12} sm={8} md={6}>
              <Statistic
                title="用户总数"
                value={usersData?.total || 0}
                prefix={<TeamOutlined />}
              />
            </Col>
            <Col xs={12} sm={8} md={6}>
              <Statistic
                title="活跃 API Key"
                value={apiKeyStats?.active || 0}
                prefix={<KeyOutlined />}
              />
            </Col>
            <Col xs={12} sm={8} md={6}>
              <Statistic
                title="已撤销 API Key"
                value={apiKeyStats?.revoked || 0}
                valueStyle={{ color: "#cf1322" }}
              />
            </Col>
          </Row>
        </Card>

        {/* 功能模块卡片 */}
        <Row gutter={[16, 16]}>
          {modules.map((module) => (
            <Col xs={24} sm={12} lg={8} key={module.title}>
              <SystemModuleCard {...module} />
            </Col>
          ))}
        </Row>
      </Space>
    </div>
  );
};
