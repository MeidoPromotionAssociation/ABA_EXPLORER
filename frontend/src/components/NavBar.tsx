import React, {useEffect} from "react";
import {Button, Layout, Menu, Tooltip} from "antd";
import {useLocation, useNavigate} from "react-router-dom";
import {useTranslation} from "react-i18next";
import {
    FolderOpenOutlined,
    HomeOutlined,
    InboxOutlined,
    SearchOutlined,
    SettingOutlined,
    TableOutlined,
    UnorderedListOutlined,
} from "@ant-design/icons";
import {Browser} from "@wailsio/runtime";
import {GitHubReleaseUrl} from "../utils/consts";
import {useVersionCheck} from "../utils/CheckUpdate";

const {Header} = Layout;

interface NavBarProps {
    onSelectFile?: () => void;
}

/**
 * NavBar 顶部导航栏
 * 菜单 key 与路由路径一致，首页用空字符串
 * Ctrl+O 打开文件，与 KCES MOD EDITOR 的快捷键保持一致
 */
const NavBar: React.FC<NavBarProps> = ({onSelectFile}) => {
    const {t} = useTranslation();
    const navigate = useNavigate();
    const location = useLocation();

    const selectedKey = location.pathname.substring(1);

    const hasUpdate = useVersionCheck();

    useEffect(() => {
        const handleKeyDown = (event: KeyboardEvent) => {
            if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "o") {
                event.preventDefault();
                onSelectFile?.();
            }
        };
        window.addEventListener("keydown", handleKeyDown);
        return () => window.removeEventListener("keydown", handleKeyDown);
    }, [onSelectFile]);

    const menuItems = [
        {key: "", icon: <HomeOutlined/>, label: t("NavBar.home")},
        {key: "container", icon: <TableOutlined/>, label: t("NavBar.container")},
        {key: "ct", icon: <UnorderedListOutlined/>, label: t("NavBar.ct")},
        {key: "search", icon: <SearchOutlined/>, label: t("NavBar.search")},
        {key: "unpacked", icon: <FolderOpenOutlined/>, label: t("NavBar.unpacked")},
        {key: "pack", icon: <InboxOutlined/>, label: t("NavBar.pack")},
        {key: "settings", icon: <SettingOutlined/>, label: t("NavBar.settings")},
    ];

    return (
        <Header style={{display: "flex", alignItems: "center", padding: "0 16px"}}>
            {hasUpdate && (
                <Button
                    type="primary"
                    danger
                    size="small"
                    style={{
                        position: 'absolute',
                        top: 0,
                        right: 0,
                        padding: '0 0px',
                        fontSize: 10,
                        lineHeight: 1,
                        zIndex: 1,
                        borderRadius: '0 0 0 4px',
                        minWidth: 20,
                        height: 17
                    }}
                    onClick={() => Browser.OpenURL(GitHubReleaseUrl)}
                >
                    NEW
                </Button>
            )}
            <div style={{flex: 1, minWidth: 0}}>
                <Menu
                    theme="dark"
                    mode="horizontal"
                    selectedKeys={[selectedKey]}
                    onClick={(event) => navigate(`/${event.key}`)}
                    items={menuItems}
                />
            </div>
            <div style={{flexShrink: 0, whiteSpace: "nowrap", marginLeft: 16}}>
                <Tooltip title={t("Common.open_file_shortcut")}>
                    <Button type="primary" onClick={onSelectFile}>
                        {t("NavBar.open_file")}
                    </Button>
                </Tooltip>
            </div>
        </Header>
    );
};

export default NavBar;
