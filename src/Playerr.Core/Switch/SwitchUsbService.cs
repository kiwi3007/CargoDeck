using System;
using System.Collections.Generic;
using System.IO;
using System.Linq;
using System.Text;
using System.Threading;
using System.Threading.Tasks;
using LibUsbDotNet;
using LibUsbDotNet.Main;
using LibUsbDotNet.Descriptors;
using Microsoft.Extensions.Logging;

namespace Playerr.Core.Switch
{
    public interface ISwitchUsbService
    {
        List<string> ScanDevices();
        Task InstallGameAsync(string filePath, string deviceId, IProgress<double> progress, CancellationToken ct);
        double CurrentProgress { get; }
        string CurrentStatus { get; }
        void CancelCurrentInstallation();
    }

    public class SwitchUsbService : ISwitchUsbService
    {
        private readonly ILogger<SwitchUsbService> _logger;
        private const int SWITCH_VENDOR_ID = 0x057E;
        private static readonly object _lock = new object();
        private bool _isBusy = false;

        public double CurrentProgress { get; private set; } = 0;
        public string CurrentStatus { get; private set; } = "Idle";
        private CancellationTokenSource _cts;
        
        public SwitchUsbService(ILogger<SwitchUsbService> logger)
        {
            _logger = logger;
        }

        public List<string> ScanDevices()
        {
            var devices = new List<string>();
            try
            {
                var allDevices = UsbDevice.AllDevices;
                _logger.LogInformation($"[SwitchUsb] Scanning. Total USB devices found by LibUsbDotNet: {allDevices.Count}");
                System.Console.WriteLine($"[SwitchUsb] Scanning. Total USB devices found by LibUsbDotNet: {allDevices.Count}");
                
                if (allDevices.Count == 0)
                {
                    _logger.LogWarning("[SwitchUsb] WARNING: No USB devices found at all. Check permissions or libusb.");
                    System.Console.WriteLine("[SwitchUsb] WARNING: No USB devices found at all. Check permissions or libusb.");
                }

                foreach (UsbRegistry registry in allDevices)
                {
                    _logger.LogInformation($"[SwitchUsb] Device: VID={registry.Vid:X4} PID={registry.Pid:X4} Name='{registry.FullName}'");
                    System.Console.WriteLine($"[SwitchUsb] Device: VID={registry.Vid:X4} PID={registry.Pid:X4} Name='{registry.FullName}'");
                    if (registry.Vid == 0x057E)
                    {
                        string modeName = registry.Pid == 0x3000 ? "DBI Backend" : "Regular/MTP";
                        _logger.LogInformation($"[SwitchUsb] MATCH FOUND: {registry.FullName} (PID={registry.Pid:X4}, Mode={modeName})");
                        System.Console.WriteLine($"[SwitchUsb] MATCH FOUND: {registry.FullName} (PID={registry.Pid:X4}, Mode={modeName})");
                        devices.Add($"Nintendo Switch ({registry.Pid:X4} - {modeName})");
                    }
                }
            }
            catch (Exception ex)
            {
                _logger.LogError(ex, "Error scanning USB devices");
                System.Console.WriteLine($"[SwitchUsb] ERROR scanning: {ex.Message}");
            }
            return devices;
        }

        public void CancelCurrentInstallation()
        {
            lock (_lock)
            {
                if (_isBusy && _cts != null)
                {
                    System.Console.WriteLine("[SwitchUsb] Cancellation requested by user.");
                    _cts.Cancel();
                }
            }
        }

        public async Task InstallGameAsync(string filePath, string deviceId, IProgress<double> progress, CancellationToken ct)
        {
            lock (_lock)
            {
                if (_isBusy) throw new Exception("An installation is already in progress.");
                _isBusy = true;
                _cts = CancellationTokenSource.CreateLinkedTokenSource(ct);
            }

            var internalCt = _cts.Token;

            CurrentProgress = 0;
            CurrentStatus = "Starting...";

            try
            {
                if (!File.Exists(filePath)) throw new FileNotFoundException("NSP file not found", filePath);

                // CRITICAL: Reset libusb context BEFORE opening device to clear stale handles
                // This is safe here because we're at the start of a new session with no pending finalizers
                System.Console.WriteLine("[SwitchUsb] Resetting libusb context to clear stale handles from previous session...");
                try 
                { 
                    UsbDevice.Exit(); 
                    await Task.Delay(500, internalCt); // Give libusb time to fully reinitialize
                } 
                catch (Exception ex) 
                { 
                    System.Console.WriteLine($"[SwitchUsb] Warning: libusb reset failed: {ex.Message}");
                }

                System.Console.WriteLine($"[SwitchUsb] Starting open attempt for VID={SWITCH_VENDOR_ID:X4}");
                
                UsbDevice usbDevice = null;
                int retryCount = 0;
                while (usbDevice == null && retryCount < 3)
                {
                    if (retryCount > 0) {
                        System.Console.WriteLine($"[SwitchUsb] Open failed, retrying... ({retryCount}/3)");
                        await Task.Delay(1000, internalCt);
                    }
                    
                    // Fallback: search manually in AllDevices and check PID
                    System.Console.WriteLine("[SwitchUsb] Scanning AllDevices for Switch...");
                    var allNow = UsbDevice.AllDevices;
                    foreach (UsbRegistry registry in allNow)
                    {
                        if (registry.Vid == SWITCH_VENDOR_ID)
                        {
                            if (registry.Pid == 0x2000)
                            {
                                CurrentStatus = "Error: Wrong Mode. Switch is in MTP/Regular mode (PID 2000). Please select 'Run DBI Backend' on the console.";
                                throw new Exception("Switch is in the wrong mode (PID 2000). Exit DBI and select 'Run DBI Backend'.");
                            }

                            System.Console.WriteLine($"[SwitchUsb] Manual find: Found {registry.FullName} (PID={registry.Pid:X4}). Attempting to open...");
                            // If we already failed once, we might need to Refresh or just rely on the Exit() above.
                            if (registry.Open(out usbDevice)) break;
                        }
                    }
                    retryCount++;
                }

                if (usbDevice == null) 
                {
                    CurrentStatus = "Error: Device not found";
                    throw new Exception("Switch device not found or could not be opened. Check connection and ensure DBI Backend is running.");
                }

                using (usbDevice)
                {
                    CurrentStatus = "Connecting...";
                    System.Console.WriteLine($"[SwitchUsb] Device opened successfully.");

                UsbEndpointReader reader = null;
                UsbEndpointWriter writer = null;

                IUsbDevice wholeUsbDevice = usbDevice as IUsbDevice;
                if (!ReferenceEquals(wholeUsbDevice, null))
                {
                    // FORCE Configuration 1
                    try 
                    {
                        _logger.LogInformation("[SwitchUsb-v2] Forcing SetConfiguration(1)...");
                        wholeUsbDevice.SetConfiguration(1);
                    }
                    catch (Exception ex)
                    {
                        _logger.LogWarning($"[SwitchUsb-v2] SetConfiguration failed: {ex.Message}");
                    }

                    // Iterate interfaces to find one with Bulk In/Out endpoints
                    _logger.LogInformation("[SwitchUsb-v2] Inspecting interfaces...");
                    for (int i = 0; i < usbDevice.Configs[0].InterfaceInfoList.Count; i++)
                    {
                        var interfaceInfo = usbDevice.Configs[0].InterfaceInfoList[i];
                        _logger.LogInformation($"[SwitchUsb-v2] Interface {i}: {interfaceInfo.Descriptor.InterfaceID} (Endpoints: {interfaceInfo.EndpointInfoList.Count})");
                        
                        // Claim this interface to inspect/use
                        if (!wholeUsbDevice.ClaimInterface(interfaceInfo.Descriptor.InterfaceID))
                        {
                            _logger.LogWarning($"[SwitchUsb-v2] Failed to claim interface {interfaceInfo.Descriptor.InterfaceID}");
                            continue;
                        }
                        
                        // Look for Endpoints
                        UsbEndpointDescriptor readEp = null;
                        UsbEndpointDescriptor writeEp = null;

                        foreach (var ep in interfaceInfo.EndpointInfoList)
                        {
                            _logger.LogInformation($"[SwitchUsb-v2]   Endpoint: {ep.Descriptor.EndpointID:X2} Type={ep.Descriptor.Attributes} MaxPacket={ep.Descriptor.MaxPacketSize}");
                            if ((ep.Descriptor.EndpointID & 0x80) == 0x80 && ep.Descriptor.Attributes == 0x02) 
                                readEp = ep.Descriptor; // Bulk IN
                            if ((ep.Descriptor.EndpointID & 0x80) == 0x00 && ep.Descriptor.Attributes == 0x02) 
                                writeEp = ep.Descriptor; // Bulk OUT
                        }

                        if (readEp != null && writeEp != null)
                        {
                            _logger.LogInformation($"[SwitchUsb-v2] Found Bulk Endpoints via Interface {interfaceInfo.Descriptor.InterfaceID}: Read={readEp.EndpointID:X2}, Write={writeEp.EndpointID:X2}");
                            reader = usbDevice.OpenEndpointReader((ReadEndpointID)readEp.EndpointID);
                            writer = usbDevice.OpenEndpointWriter((WriteEndpointID)writeEp.EndpointID);
                            break; // Found valid pair
                        }
                        
                        wholeUsbDevice.ReleaseInterface(interfaceInfo.Descriptor.InterfaceID);
                    }
                }

                if (reader == null || writer == null)
                {
                    // Fallback to hardcoded if dynamic fails (unlikely if logic is correct)
                    _logger.LogWarning("[SwitchUsb-v2] Dynamic discovery failed. Trying default 0x81/0x01.");
                    reader = usbDevice.OpenEndpointReader(ReadEndpointID.Ep01);
                    writer = usbDevice.OpenEndpointWriter(WriteEndpointID.Ep01);
                }
                
                if (reader == null || writer == null)
                {
                    throw new Exception("Could not open USB endpoints. Ensure Tinfoil/DBI is in USB Install mode.");
                }

                try 
                {
                    CurrentStatus = "Waiting for Switch request...";
                    bool finishedNormally = await PerformTinfoilInstall(wholeUsbDevice, writer, reader, filePath, progress, internalCt);
                    
                    if (finishedNormally)
                    {
                        CurrentStatus = "Installation Complete";
                        CurrentProgress = 100;
                    }
                    else
                    {
                        CurrentStatus = "Installation Aborted by Console";
                    }
                }
                catch (Exception ex)
                {
                    CurrentStatus = $"Error: {ex.Message}";
                    _logger.LogError(ex, "Installation failed");
                    throw;
                }
                finally
                {
                    if (!ReferenceEquals(wholeUsbDevice, null))
                    {
                         try { wholeUsbDevice.ReleaseInterface(0); } catch {}
                         try { wholeUsbDevice.ReleaseInterface(1); } catch {}
                    }
                    if (usbDevice.IsOpen) usbDevice.Close();
                }
            }
            }
            finally
            {
                lock (_lock)
                {
                    _isBusy = false;
                }
            }
        }

        // DBI Constants (from dbibackend)
        private const int CMD_TYPE_REQUEST = 0;
        private const int CMD_TYPE_RESPONSE = 1;
        private const int CMD_TYPE_ACK = 2;

        private const int CMD_ID_EXIT = 0;
        private const int CMD_ID_FILE_RANGE = 2;
        private const int CMD_ID_LIST = 3;

        private void SendExitResponse(UsbEndpointWriter writer)
        {
            try
            {
                var exitResp = new List<byte>();
                exitResp.AddRange(Encoding.ASCII.GetBytes("DBI0"));
                exitResp.AddRange(BitConverter.GetBytes(CMD_TYPE_RESPONSE));
                exitResp.AddRange(BitConverter.GetBytes(CMD_ID_EXIT));
                exitResp.AddRange(BitConverter.GetBytes(0));
                writer.Write(exitResp.ToArray(), 1000, out _);
                _logger.LogInformation("[DBI] Exit signal sent to Switch.");
            }
            catch (Exception ex)
            {
                _logger.LogWarning($"[DBI] Failed to send exit signal: {ex.Message}");
            }
        }

        private async Task<bool> PerformTinfoilInstall(IUsbDevice usbDevice, UsbEndpointWriter writer, UsbEndpointReader reader, string filePath, IProgress<double> progress, CancellationToken ct)
        {
             _logger.LogInformation("[SwitchUsb-v2] Waiting for Switch request (DBI Mode)...");
             
             byte[] buffer = new byte[0x200]; 
             long totalSize = new FileInfo(filePath).Length;
             
             try
             {
                 while (!ct.IsCancellationRequested)
                 {
                     int bytesRead;
                     ErrorCode ec = reader.Read(buffer, 100, out bytesRead);
                     
                     if (bytesRead > 0)
                     {
                         string magic = Encoding.ASCII.GetString(buffer, 0, 4);
                         if (magic == "DBI0")
                         {
                             int cmdType = BitConverter.ToInt32(buffer, 4);
                             int cmdId = BitConverter.ToInt32(buffer, 8);
                             int dataSize = BitConverter.ToInt32(buffer, 12);
                             
                             _logger.LogInformation($"[DBI] Recv: Type={cmdType} ID={cmdId} DataSize={dataSize}");
    
                             if (cmdType == CMD_TYPE_REQUEST)
                             {
                                 switch (cmdId)
                                 {
                                     case CMD_ID_EXIT:
                                         _logger.LogInformation("[DBI) Exit requested by Switch.");
                                         SendExitResponse(writer);
                                         return false;
    
                                     case CMD_ID_LIST:
                                         _logger.LogInformation("[DBI] List requested.");
                                         await HandleDbiList(reader, writer, filePath, ct);
                                         break;
    
                                     case CMD_ID_FILE_RANGE:
                                          await HandleDbiFileRange(reader, writer, filePath, dataSize, progress, totalSize, buffer, bytesRead, ct);
                                          break;
                                 }
                             }
                         }
                         else if (magic == "TUC0")
                         {
                             _logger.LogWarning("[SwitchUsb] TUC0 detected (Legacy Tinfoil). Not implemented.");
                         }
                     }
                     
                     await Task.Delay(5, ct);
                 }
                 return true;
             }
             catch (OperationCanceledException)
             {
                 _logger.LogInformation("[DBI] Cancellation detected from Host. Sending graceful EXIT to Switch...");
                 SendExitResponse(writer);
                 
                 // Give the Switch time to process the exit signal and return to menu
                 // This delay is critical - without it, the Switch may freeze
                 await Task.Delay(500);
                 
                 throw;
             }
        }

        private async Task HandleDbiList(UsbEndpointReader reader, UsbEndpointWriter writer, string filePath, CancellationToken ct)
        {
             var fileName = Path.GetFileName(filePath);
             var fileBytes = Encoding.UTF8.GetBytes(fileName + "\n"); // Must end with newline? Python code: nsp_path_list += k + '\n'
             var listLen = fileBytes.Length;

             // 1. Send Response Header
             // Struct: 4s I I I (Magic, Type, CmdId, Len)
             var header = new List<byte>();
             header.AddRange(Encoding.ASCII.GetBytes("DBI0"));
             header.AddRange(BitConverter.GetBytes(CMD_TYPE_RESPONSE));
             header.AddRange(BitConverter.GetBytes(CMD_ID_LIST));
             header.AddRange(BitConverter.GetBytes(listLen));
             
             int sent;
             writer.Write(header.ToArray(), 1000, out sent);
             _logger.LogInformation($"[DBI] Sent List Header ({sent} bytes). Waiting for ACK...");

             if (listLen > 0)
             {
                 // 2. Wait for ACK
                 byte[] ackBuf = new byte[16];
                 int ackRead;
                 ErrorCode ackEc = reader.Read(ackBuf, 5000, out ackRead); // Long timeout for ACK
                 
                 if (ackEc != ErrorCode.None || ackRead == 0)
                 {
                     _logger.LogError($"[DBI] Failed to receive ACK for List: {ackEc}");
                     return;
                 }
                 _logger.LogInformation("[DBI] ACK received. Sending Data...");
                 
                 // 3. Send Data
                 writer.Write(fileBytes, 2000, out sent);
                 _logger.LogInformation($"[DBI] Sent List Data ({sent} bytes).");
             }
        }

        private async Task HandleDbiFileRange(UsbEndpointReader reader, UsbEndpointWriter writer, string filePath, int headerSize, IProgress<double> progress, long totalSize, byte[] initialBuffer, int initialRead, CancellationToken ct)
        {
            // 1. Send ACK (with headerSize)
            var ack = new List<byte>();
            ack.AddRange(Encoding.ASCII.GetBytes("DBI0"));
            ack.AddRange(BitConverter.GetBytes(CMD_TYPE_ACK));
            ack.AddRange(BitConverter.GetBytes(CMD_ID_FILE_RANGE));
            ack.AddRange(BitConverter.GetBytes(headerSize)); 

            int sent;
            writer.Write(ack.ToArray(), 1000, out sent);

            // 2. Read Range Header (headerSize bytes)
            byte[] rangeHeader = new byte[headerSize];
            int alreadyBuffered = Math.Max(0, initialRead - 16);
            if (alreadyBuffered > 0)
            {
                Array.Copy(initialBuffer, 16, rangeHeader, 0, Math.Min(alreadyBuffered, headerSize));
            }
            
            if (alreadyBuffered < headerSize)
            {
                int remainingHeader = headerSize - alreadyBuffered;
                byte[] extra = new byte[remainingHeader];
                int read;
                reader.Read(extra, 2000, out read);
                Array.Copy(extra, 0, rangeHeader, alreadyBuffered, read);
            }

            // 3. Parse Range Header (Python: I Q I ...)
            uint rangeSize = BitConverter.ToUInt32(rangeHeader, 0); 
            long rangeOffset = BitConverter.ToInt64(rangeHeader, 4);
            uint nameLen = BitConverter.ToUInt32(rangeHeader, 12);
            string requestedName = Encoding.UTF8.GetString(rangeHeader, 16, Math.Min((int)nameLen, headerSize - 16));
            
            _logger.LogInformation($"[DBI] Range Request: '{requestedName}' Offset={rangeOffset}, Size={rangeSize}");

            // 4. Send Response Header (Type=RESPONSE, Size=rangeSize)
            var responseHeader = new List<byte>();
            responseHeader.AddRange(Encoding.ASCII.GetBytes("DBI0"));
            responseHeader.AddRange(BitConverter.GetBytes(CMD_TYPE_RESPONSE));
            responseHeader.AddRange(BitConverter.GetBytes(CMD_ID_FILE_RANGE));
            responseHeader.AddRange(BitConverter.GetBytes((int)rangeSize)); 

            writer.Write(responseHeader.ToArray(), 1000, out sent);

            // 5. Read ACK (16 bytes)
            byte[] finalAck = new byte[16];
            int finalRead;
            _ = reader.Read(finalAck, 5000, out finalRead);

            // 6. Send File Content
            using (var fs = new FileStream(filePath, FileMode.Open, FileAccess.Read, FileShare.Read))
            {
                fs.Seek(rangeOffset, SeekOrigin.Begin);
                
                byte[] chunk = new byte[BUFFER_SEGMENT_DATA_SIZE]; 
                long remaining = rangeSize;
                
                while (remaining > 0 && !ct.IsCancellationRequested)
                {
                    int toRead = (int)Math.Min(chunk.Length, remaining);
                    int actuallyRead = await fs.ReadAsync(chunk, 0, toRead, ct);
                    
                    ErrorCode ec = writer.Write(chunk, 0, actuallyRead, 10000, out sent);
                    if (ec != ErrorCode.None)
                    {
                        throw new Exception($"USB Write Error: {ec}");
                    }
                    
                    remaining -= sent;
                    long uploaded = rangeOffset + (rangeSize - remaining);
                    double p = (double)uploaded / totalSize * 100.0;
                    CurrentProgress = p;
                    progress.Report(p);
                    CurrentStatus = $"Transferring: {p:F1}%";
                }

                // If cancelled mid-transfer, the exit signal in the catch block will handle cleanup
                // No need to send dummy bytes - the 500ms delay gives the Switch time to process the exit
            }
        }
        
        private const int BUFFER_SEGMENT_DATA_SIZE = 0x100000; // 1MB
    }
}
