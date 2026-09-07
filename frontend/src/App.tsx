// App.tsx
import React, {useEffect, useState} from "react";
import {Route, Routes} from "react-router-dom";
import {App as AntdApp, ConfigProvider, Layout, theme} from "antd";
import {useTranslation} from "react-i18next";
import {Events} from "@wailsio/runtime";
import NavBar from "./components/NavBar";
import HomePage from "./components/HomePage";
import ContainerPage from "./components/ContainerPage";
import CtPage from "./components/CtPage";
import SearchPage from "./components/SearchPage";
import UnpackedPage from "./components/UnpackedPage";
import PackPage from "./components/PackPage";
import SettingsPage from "./components/SettingsPage";
import {DefaultThemeColor, useDarkMode, useThemeColor} from "./hooks/themeSwitch";
import useFileOpener from "./hooks/fileOpener";
import {bindMessage} from "./utils/feedback";
import {FileDroppedEvent, ProtocolOpenEvent} from "./utils/consts";
import {getAntdLocale} from "./utils/i18n";
import {App as AppService} from "../bindings/github.com/MeidoPromotionAssociation/ABA_EXPLORER/internal";
import DisclaimerDialog from "./components/DisclaimerDialog.tsx";
import {DisclaimerAgreedKey} from "./utils/LocalStorageKeys.ts";

const {Content} = Layout;


// MessageBinder 把组件树内（可消费主题上下文）的 message 实例绑定到全局桥
const MessageBinder: React.FC = () => {
    const {message} = AntdApp.useApp();
    useEffect(() => {
        bindMessage(message);
    }, [message]);
    return null;
};

const App: React.FC = () => {
    const isDarkMode = useDarkMode();
    const [themeColor] = useThemeColor();
    const {openPath, selectAndOpen} = useFileOpener();
    const [showDisclaimer, setShowDisclaimer] = useState(() => {
        return localStorage.getItem(DisclaimerAgreedKey) !== 'true';
    });

    // 用户同意免责声明
    const handleAgreeDisclaimer = () => {
        setShowDisclaimer(false);
        localStorage.setItem(DisclaimerAgreedKey, 'true');
    };

    // 通过文件关联启动时打开传入的文件
    useEffect(() => {
        let cancelled = false;
        (async () => {
            try {
                const path = await AppService.StartupFile();
                if (!cancelled && path) await openPath(path);
            } catch (error) {
                console.error("open startup file failed:", error);
            }
        })();
        return () => {
            cancelled = true;
        };
        // 只在挂载时执行一次，openPath 的身份变化不应重新打开启动文件
    }, []);

    // 用户拖放文件
    useEffect(() => {
        const off = Events.On(FileDroppedEvent, async (event: any) => {
            const data = event?.data;
            const path = Array.isArray(data) ? data[0] : data;
            if (typeof path === "string" && path) await openPath(path);
        });
        return () => {
            off();
        };
    }, [openPath]);

    // 开启单实例后，另一次启动（协议唤起或文件关联双击）会把目标路径转交到这里
    // With single instance on, another launch — a protocol invocation or an association double-click — hands its target here
    useEffect(() => {
        const off = Events.On(ProtocolOpenEvent, async (event: any) => {
            const data = event?.data;
            const path = Array.isArray(data) ? data[0] : data;
            if (typeof path === "string" && path) await openPath(path);
        });
        return () => {
            off();
        };
    }, [openPath]);

    // 订阅语言变化，切换语言后重新解析 antd 的 locale
    useTranslation();

    return (
        <ConfigProvider
            locale={getAntdLocale()}
            theme={{
                algorithm: isDarkMode ? theme.darkAlgorithm : theme.defaultAlgorithm,
                token: {colorPrimary: themeColor ?? DefaultThemeColor},
            }}
        >
            <AntdApp component={false}>
                <MessageBinder/>
                <DisclaimerDialog visible={showDisclaimer} onAgree={handleAgreeDisclaimer}/>
                {!showDisclaimer && (
                    <Layout style={{height: "100vh"}}>
                        <NavBar onSelectFile={selectAndOpen}/>
                        <Content style={{padding: 16, overflow: "hidden", display: "flex", minHeight: 0}}>
                            <Routes>
                                <Route path="/" element={<HomePage/>}/>
                                <Route path="/container" element={<ContainerPage/>}/>
                                <Route path="/ct" element={<CtPage/>}/>
                                <Route path="/search" element={<SearchPage/>}/>
                                <Route path="/unpacked" element={<UnpackedPage/>}/>
                                <Route path="/pack" element={<PackPage/>}/>
                                <Route path="/settings" element={<SettingsPage/>}/>
                            </Routes>
                        </Content>
                    </Layout>
                )}
            </AntdApp>
        </ConfigProvider>
    );
};

export default App;
