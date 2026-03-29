// swift-tools-version: 5.9
import PackageDescription

let package = Package(
    name: "SSOSDK",
    platforms: [.iOS(.v15), .macOS(.v12)],
    products: [
        .library(name: "SSOSDK", targets: ["SSOSDK"]),
    ],
    dependencies: [],
    targets: [
        .target(name: "SSOSDK", dependencies: []),
        .testTarget(name: "SSOSDKTests", dependencies: ["SSOSDK"]),
    ]
)
