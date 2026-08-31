import React, {useState} from "react";
import {Button, Card, Col, Empty, List, Row, Space, Typography, theme} from "antd";
import {
    FolderOpenOutlined,
    InboxOutlined,
    TableOutlined,
    UnorderedListOutlined,
} from "@ant-design/icons";
import {useTranslation} from "react-i18next";
import {useNavigate} from "react-router-dom";
import {App as AppService} from "../../bindings/github.com/MeidoPromotionAssociation/ABA_EXPLORER/internal";
import useFileOpener from "../hooks/fileOpener";
import {clearRecentFiles, readRecentFiles, setUnpackedDir} from "../hooks/workspace";
import {ContainerFilter, CtFilter} from "../utils/consts";
import {appMessage as message, describeError} from "../utils/feedback";
import {baseName, dirName} from "../utils/format";

const {Title, Text, Paragraph} = Typography;

/**
 * HomePage 入口页
 * 四个入口分别对应打开容器、打开内容表、浏览解包目录和打包，
 * 下方列出最近打开过的路径，点击即重新打开
 */
const HomePage: React.FC = () => {
    const {t} = useTranslation();
    const {token} = theme.useToken();
    const navigate = useNavigate();
    const {openPath} = useFileOpener();
    const [recent, setRecent] = useState<string[]>(() => readRecentFiles());

    const pickFile = async (filter: string, label: string) => {
        try {
            const path = await AppService.SelectFile(filter, label);
            if (path) {
                await openPath(path);
                setRecent(readRecentFiles());
            }
        } catch (error) {
            message.error(describeError(error));
        }
    };

    const pickUnpackedDir = async () => {
        try {
            const dir = await AppService.SelectDirectory(t("HomePage.choose_unpacked_dir"));
            if (dir) {
                setUnpackedDir(dir);
                navigate("/unpacked");
            }
        } catch (error) {
            message.error(describeError(error));
        }
    };

    const entries = [
        {
            key: "container",
            icon: <TableOutlined style={{fontSize: 26, color: token.colorPrimary}}/>,
            title: t("HomePage.open_container"),
            description: t("HomePage.open_container_desc"),
            action: () => pickFile(ContainerFilter, t("Common.container_files")),
        },
        {
            key: "ct",
            icon: <UnorderedListOutlined style={{fontSize: 26, color: token.colorPrimary}}/>,
            title: t("HomePage.open_ct"),
            description: t("HomePage.open_ct_desc"),
            action: () => pickFile(CtFilter, t("Common.ct_files")),
        },
        {
            key: "unpacked",
            icon: <FolderOpenOutlined style={{fontSize: 26, color: token.colorPrimary}}/>,
            title: t("HomePage.open_unpacked"),
            description: t("HomePage.open_unpacked_desc"),
            action: pickUnpackedDir,
        },
        {
            key: "pack",
            icon: <InboxOutlined style={{fontSize: 26, color: token.colorPrimary}}/>,
            title: t("HomePage.pack"),
            description: t("HomePage.pack_desc"),
            action: () => navigate("/pack"),
        },
    ];

    return (
        <div style={{flex: 1, minWidth: 0, overflow: "auto"}}>
            <Title level={3} style={{marginTop: 0}}>{t("HomePage.title")}</Title>
            <Paragraph type="secondary" style={{marginBottom: 20}}>
                {t("HomePage.subtitle")}
            </Paragraph>

            <Row gutter={[16, 16]}>
                {entries.map((entry) => (
                    <Col key={entry.key} xs={24} sm={12} lg={6}>
                        <Card hoverable onClick={entry.action} style={{height: "100%"}}>
                            <Space direction="vertical" size={6}>
                                {entry.icon}
                                <Text strong>{entry.title}</Text>
                                <Text type="secondary" style={{fontSize: 12}}>{entry.description}</Text>
                            </Space>
                        </Card>
                    </Col>
                ))}
            </Row>

            <Card
                style={{marginTop: 20}}
                title={t("HomePage.recent")}
                extra={
                    recent.length > 0 && (
                        <Button
                            size="small"
                            onClick={() => {
                                clearRecentFiles();
                                setRecent([]);
                            }}
                        >
                            {t("HomePage.clear_recent")}
                        </Button>
                    )
                }
            >
                {recent.length === 0 ? (
                    <Empty description={t("HomePage.no_recent")} image={Empty.PRESENTED_IMAGE_SIMPLE}/>
                ) : (
                    <List
                        size="small"
                        dataSource={recent}
                        renderItem={(path) => (
                            <List.Item
                                style={{cursor: "pointer"}}
                                onClick={async () => {
                                    await openPath(path);
                                    setRecent(readRecentFiles());
                                }}
                            >
                                <List.Item.Meta
                                    title={<Text>{baseName(path)}</Text>}
                                    description={<Text type="secondary" style={{fontSize: 12}}>{dirName(path)}</Text>}
                                />
                            </List.Item>
                        )}
                    />
                )}
            </Card>
        </div>
    );
};

export default HomePage;
