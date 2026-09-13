import AppIntents
import AppKit
import Foundation
import ExtensionFoundation

@main
struct YourDeskSiriExtension: AppIntentsExtension {}

struct BridgeError: LocalizedError {
    let message: String
    var errorDescription: String? { message }
}
struct Bridge {
    struct Ticket: Decodable { let url: String; let token: String; let pid: Int32 }
    struct Reply: Decodable {
        var message: String?
        var error: String?
        var sites: [Device]?
    }
    static var ticketURL: URL {
        URL(fileURLWithPath: NSHomeDirectoryForUser(NSUserName()) ?? NSHomeDirectory())
            .appendingPathComponent("Library/Application Support/YourDesk/siri-bridge.json")
    }
    @MainActor static func launch() async throws {
        // appex 位於 YourDesk.app/Contents/Extensions；限定開啟同一份發行包。
        let app = Bundle.main.bundleURL.deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
        guard app.pathExtension == "app" else { throw BridgeError(message: "請從完整 YourDesk.app 使用 Siri") }
        let config = NSWorkspace.OpenConfiguration()
        config.activates = true
        _ = try await NSWorkspace.shared.openApplication(at: app, configuration: config)
    }
    static func call(_ action: String, id: String? = nil) async throws -> Reply {
        var ticket = try? JSONDecoder().decode(Ticket.self, from: Data(contentsOf: ticketURL))
        if ticket == nil || kill(ticket!.pid, 0) != 0 {
            try await launch()
            for _ in 0..<100 {
                try await Task.sleep(for: .milliseconds(100))
                ticket = try? JSONDecoder().decode(Ticket.self, from: Data(contentsOf: ticketURL))
                if let ticket, kill(ticket.pid, 0) == 0 { break }
            }
        }
        guard let ticket, let url = URL(string: ticket.url), url.scheme == "http", url.host == "127.0.0.1", url.path == "/automation", url.user == nil, url.password == nil else {
            throw BridgeError(message: "YourDesk 尚未啟動完成，請稍後再試")
        }
        var request = URLRequest(url: url, timeoutInterval: 15)
        request.httpMethod = "POST"
        request.setValue("Bearer \(ticket.token)", forHTTPHeaderField: "Authorization")
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        var body = ["action": action]
        if let id { body["id"] = id }
        request.httpBody = try JSONEncoder().encode(body)
        let config = URLSessionConfiguration.ephemeral
        config.connectionProxyDictionary = [:]
        let session = URLSession(configuration: config, delegate: NoRedirect(), delegateQueue: nil)
        defer { session.invalidateAndCancel() }
        let (data, response) = try await session.data(for: request)
        let reply = try JSONDecoder().decode(Reply.self, from: data)
        guard (response as? HTTPURLResponse)?.statusCode == 200 else { throw BridgeError(message: reply.error ?? "YourDesk 操作失敗") }
        return reply
    }
    private final class NoRedirect: NSObject, URLSessionTaskDelegate, @unchecked Sendable {
        func urlSession(_ session: URLSession, task: URLSessionTask, willPerformHTTPRedirection response: HTTPURLResponse, newRequest request: URLRequest, completionHandler: @escaping (URLRequest?) -> Void) { completionHandler(nil) }
    }
}

struct Device: AppEntity, Decodable {
    static var typeDisplayRepresentation: TypeDisplayRepresentation = "YourDesk 電腦"
    static var defaultQuery = DeviceQuery()
    let id: String
    let name: String
    var displayRepresentation: DisplayRepresentation { DisplayRepresentation(title: "\(name)") }
}
struct DeviceQuery: EntityStringQuery {
    func entities(for identifiers: [String]) async throws -> [Device] {
        try await suggestedEntities().filter { identifiers.contains($0.id) }
    }
    func suggestedEntities() async throws -> [Device] { try await Bridge.call("sites").sites ?? [] }
    func entities(matching string: String) async throws -> [Device] {
        try await suggestedEntities().filter { $0.name.localizedCaseInsensitiveContains(string) }
    }
}
struct OpenYourDesk: AppIntent {
    static var title: LocalizedStringResource = "開啟 YourDesk"
    static var authenticationPolicy: IntentAuthenticationPolicy = .requiresLocalDeviceAuthentication
    func perform() async throws -> some IntentResult & ProvidesDialog {
        let reply = try await Bridge.call("open")
        return .result(dialog: "\(reply.message ?? "已開啟 YourDesk")")
    }
}
struct ConnectDevice: AppIntent {
    static var title: LocalizedStringResource = "連線到電腦"
    static var authenticationPolicy: IntentAuthenticationPolicy = .requiresLocalDeviceAuthentication
    @Parameter(title: "電腦") var device: Device
    static var parameterSummary: some ParameterSummary { Summary("使用 YourDesk 連線到 \(\.$device)") }
    func perform() async throws -> some IntentResult & ProvidesDialog {
        let reply = try await Bridge.call("connect", id: device.id)
        return .result(dialog: "\(reply.message ?? "已開始連線")")
    }
}
struct DisconnectDevice: AppIntent {
    static var title: LocalizedStringResource = "中斷連線"
    static var authenticationPolicy: IntentAuthenticationPolicy = .requiresLocalDeviceAuthentication
    @Parameter(title: "電腦") var device: Device?
    static var parameterSummary: some ParameterSummary { Summary("中斷 YourDesk 連線 \(\.$device)") }
    func perform() async throws -> some IntentResult & ProvidesDialog {
        let reply = try await Bridge.call("disconnect", id: device?.id)
        return .result(dialog: "\(reply.message ?? "已中斷連線")")
    }
}
struct ConnectionStatus: AppIntent {
    static var title: LocalizedStringResource = "查詢連線狀態"
    static var authenticationPolicy: IntentAuthenticationPolicy = .requiresLocalDeviceAuthentication
    @Parameter(title: "電腦") var device: Device?
    func perform() async throws -> some IntentResult & ReturnsValue<String> & ProvidesDialog {
        let message = try await Bridge.call("status", id: device?.id).message ?? "目前沒有連線"
        return .result(value: message, dialog: "\(message)")
    }
}
struct YourDeskShortcuts: AppShortcutsProvider {
    static var appShortcuts: [AppShortcut] {
        AppShortcut(intent: OpenYourDesk(), phrases: ["開啟 \(.applicationName)"], shortTitle: "開啟 YourDesk", systemImageName: "desktopcomputer")
        AppShortcut(intent: ConnectDevice(), phrases: ["使用 \(.applicationName) 連線到 \(\.$device)"], shortTitle: "連線到電腦", systemImageName: "network")
        AppShortcut(intent: DisconnectDevice(), phrases: ["中斷 \(.applicationName) 連線"], shortTitle: "中斷連線", systemImageName: "xmark.circle")
        AppShortcut(intent: ConnectionStatus(), phrases: ["查詢 \(.applicationName) 連線狀態"], shortTitle: "查詢連線狀態", systemImageName: "info.circle")
    }
}

// 新版 Siri AI 的通用內容開啟 schema；不把「開啟站台」偽裝成連線操作。
@available(macOS 27.0, *)
@AppIntent(schema: .system.open)
struct OpenSavedDevice: OpenIntent {
    static var title: LocalizedStringResource = "開啟已儲存電腦"
    static var authenticationPolicy: IntentAuthenticationPolicy = .requiresLocalDeviceAuthentication
    @Parameter(title: "電腦") var target: Device
    func perform() async throws -> some IntentResult {
        _ = try await Bridge.call("show-site", id: target.id)
        return .result()
    }
}
