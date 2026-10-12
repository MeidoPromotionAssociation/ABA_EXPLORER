import React, {useState} from "react";
import {Button, Card, Col, Empty, Listy, Row, Space, Tag, theme, Typography} from "antd";
import {
    FolderOpenOutlined,
    InboxOutlined,
    SearchOutlined,
    TableOutlined,
    UnorderedListOutlined,
} from "@ant-design/icons";
import {useTranslation} from "react-i18next";
import {useNavigate} from "react-router-dom";
import {App as AppService} from "../../bindings/github.com/MeidoPromotionAssociation/ABA_EXPLORER/internal";
import useFileOpener from "../hooks/fileOpener";
import {clearRecentFiles, readRecentFiles, setUnpackedDir, type RecentEntry, type RecentKind} from "../hooks/workspace";
import {ContainerFilter, CtFilter} from "../utils/consts";
import {appMessage as message, describeError} from "../utils/feedback";
import {baseName, dirName} from "../utils/format";

const {Title, Text, Paragraph} = Typography;

// 最近打开条目的种类图标，与上方入口卡片的图标保持一致
// Icons for recent entry kinds, kept in sync with the entry cards above
const RecentKindIcons: Record<RecentKind, React.ReactNode> = {
    container: <TableOutlined/>,
    ct: <UnorderedListOutlined/>,
    unpacked: <FolderOpenOutlined/>,
};

/**
 * HomePage 入口页
 * 五个入口分别对应打开容器、打开内容表、全局搜索、浏览解包目录和打包，
 * 下方列出最近打开过的容器、内容表与解包目录，点击即重新打开
 */
const HomePage: React.FC = () => {
    const {t} = useTranslation();
    const {token} = theme.useToken();
    const navigate = useNavigate();
    const {openPath} = useFileOpener();
    const [recent, setRecent] = useState<RecentEntry[]>(() => readRecentFiles());

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
            key: "search",
            icon: <SearchOutlined style={{fontSize: 26, color: token.colorPrimary}}/>,
            title: t("HomePage.search"),
            description: t("HomePage.search_desc"),
            action: () => navigate("/search"),
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
                    <Col key={entry.key} xs={24} sm={12} lg={8}>
                        <Card hoverable onClick={entry.action} style={{height: "100%"}}>
                            <Space vertical size={6}>
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
                    <Listy
                        items={recent}
                        rowKey={(entry) => entry.path}
                        classNames={{root: "recent-list"}}
                        styles={{item: {padding: 0}}}
                        itemRender={(entry) => (
                            <div
                                style={{cursor: "pointer", padding: "8px 16px"}}
                                onClick={async () => {
                                    await openPath(entry.path);
                                    setRecent(readRecentFiles());
                                }}
                            >
                                <Space size={8} style={{marginBottom: 8}}>
                                    <span style={{color: token.colorPrimary, display: "inline-flex"}}>
                                        {RecentKindIcons[entry.kind]}
                                    </span>
                                    <Text>{baseName(entry.path)}</Text>
                                    {/* 种类标签复用导航栏的名称，三种条目在四个语言里已有译文 */}
                                    <Tag style={{marginInlineEnd: 0}}>{t(`NavBar.${entry.kind}`)}</Tag>
                                </Space>
                                <div>
                                    <Text type="secondary" style={{fontSize: 12}}>{dirName(entry.path)}</Text>
                                </div>
                            </div>
                        )}
                    />
                )}
            </Card>
        </div>
    );
};

export default HomePage;
