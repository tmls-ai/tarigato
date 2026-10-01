// Encode the demo's rendered terminal frames using macOS' built-in video encoder.
import AVFoundation
import CoreGraphics
import Foundation
import ImageIO

struct EncodeError: Error, CustomStringConvertible {
    let description: String
    init(_ description: String) { self.description = description }
}

func loadImage(_ url: URL) throws -> CGImage {
    guard let source = CGImageSourceCreateWithURL(url as CFURL, nil),
          let image = CGImageSourceCreateImageAtIndex(source, 0, nil) else {
        throw EncodeError("Cannot read PNG: \(url.lastPathComponent)")
    }
    return image
}

func encode() async throws {
    let args = CommandLine.arguments
    guard args.count == 4, let fps = Int32(args[3]), (1...60).contains(fps) else {
        throw EncodeError("Usage: swift encode.swift FRAME_DIRECTORY OUTPUT.mp4 FPS (1–60)")
    }
    let output = URL(fileURLWithPath: args[2])
    guard !FileManager.default.fileExists(atPath: output.path) else {
        throw EncodeError("Output already exists: \(output.path)")
    }
    let frames = try FileManager.default.contentsOfDirectory(
        at: URL(fileURLWithPath: args[1]), includingPropertiesForKeys: nil
    ).filter { $0.pathExtension.lowercased() == "png" }
        .sorted { $0.lastPathComponent < $1.lastPathComponent }
    guard let first = frames.first else { throw EncodeError("No PNG frames found") }
    let sample = try loadImage(first)
    let width = sample.width, height = sample.height
    guard width % 2 == 0, height % 2 == 0 else {
        throw EncodeError("H.264 requires even frame dimensions; received \(width)×\(height)")
    }

    let writer = try AVAssetWriter(outputURL: output, fileType: .mp4)
    writer.shouldOptimizeForNetworkUse = true
    let input = AVAssetWriterInput(mediaType: .video, outputSettings: [
        AVVideoCodecKey: AVVideoCodecType.h264,
        AVVideoWidthKey: width, AVVideoHeightKey: height,
        AVVideoColorPropertiesKey: [
            AVVideoColorPrimariesKey: AVVideoColorPrimaries_ITU_R_709_2,
            AVVideoTransferFunctionKey: kCVImageBufferTransferFunction_sRGB,
            AVVideoYCbCrMatrixKey: AVVideoYCbCrMatrix_ITU_R_709_2
        ],
        AVVideoCompressionPropertiesKey: [
            AVVideoAverageBitRateKey: 2_000_000,
            AVVideoExpectedSourceFrameRateKey: fps,
            AVVideoMaxKeyFrameIntervalKey: fps * 2
        ]
    ])
    let buffers = AVAssetWriterInputPixelBufferAdaptor(
        assetWriterInput: input, sourcePixelBufferAttributes: [
            kCVPixelBufferPixelFormatTypeKey as String: kCVPixelFormatType_32BGRA,
            kCVPixelBufferWidthKey as String: width,
            kCVPixelBufferHeightKey as String: height,
            kCVPixelBufferCGImageCompatibilityKey as String: true,
            kCVPixelBufferCGBitmapContextCompatibilityKey as String: true
        ])
    guard writer.canAdd(input) else { throw EncodeError("Cannot configure H.264 encoder") }
    writer.add(input)
    guard writer.startWriting() else { throw writer.error ?? EncodeError("Cannot start encoder") }
    var complete = false
    defer { if !complete { writer.cancelWriting(); try? FileManager.default.removeItem(at: output) } }
    writer.startSession(atSourceTime: .zero)
    guard let pool = buffers.pixelBufferPool, let space = CGColorSpace(name: CGColorSpace.sRGB) else {
        throw EncodeError("Cannot allocate video buffers")
    }
    for (index, frame) in frames.enumerated() {
        let image = try loadImage(frame)
        guard image.width == width, image.height == height else {
            throw EncodeError("Frame dimensions changed: \(frame.lastPathComponent)")
        }
        while !input.isReadyForMoreMediaData {
            guard writer.status == .writing else { throw writer.error ?? EncodeError("Encoder stopped") }
            try await Task.sleep(nanoseconds: 1_000_000)
        }
        var optionalBuffer: CVPixelBuffer?
        guard CVPixelBufferPoolCreatePixelBuffer(nil, pool, &optionalBuffer) == kCVReturnSuccess,
              let buffer = optionalBuffer else { throw EncodeError("Cannot allocate frame buffer") }
        CVBufferSetAttachment(buffer, kCVImageBufferCGColorSpaceKey, space, .shouldPropagate)
        CVPixelBufferLockBaseAddress(buffer, [])
        defer { CVPixelBufferUnlockBaseAddress(buffer, []) }
        guard let context = CGContext(
            data: CVPixelBufferGetBaseAddress(buffer), width: width, height: height,
            bitsPerComponent: 8, bytesPerRow: CVPixelBufferGetBytesPerRow(buffer), space: space,
            bitmapInfo: CGImageAlphaInfo.premultipliedFirst.rawValue | CGBitmapInfo.byteOrder32Little.rawValue
        ) else { throw EncodeError("Cannot draw frame") }
        context.draw(image, in: CGRect(x: 0, y: 0, width: width, height: height))
        guard buffers.append(buffer, withPresentationTime: CMTime(value: Int64(index), timescale: fps)) else {
            throw writer.error ?? EncodeError("Cannot append frame")
        }
    }
    writer.endSession(atSourceTime: CMTime(value: Int64(frames.count), timescale: fps))
    input.markAsFinished()
    await writer.finishWriting()
    guard writer.status == .completed else { throw writer.error ?? EncodeError("Cannot finish video") }
    complete = true
    print("Encoded \(frames.count) frames, \(width)×\(height), \(fps) fps → \(output.path)")
}

do { try await encode() } catch {
    FileHandle.standardError.write(Data("encode: \(error)\n".utf8))
    exit(1)
}
