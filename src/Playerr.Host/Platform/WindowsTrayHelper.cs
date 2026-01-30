#if WINDOWS
using System;
using System.Drawing;
using System.Runtime.InteropServices;
using System.Windows.Forms;
using Photino.NET;

namespace Playerr.Host.Platform
{
    public class WindowsTrayHelper : IDisposable
    {
        private NotifyIcon _notifyIcon;
        private PhotinoWindow _window;
        private bool _isExplicitExit = false;

        // P/Invoke for hiding/showing the window
        [DllImport("user32.dll")]
        private static extern bool ShowWindow(IntPtr hWnd, int nCmdShow);
        private const int SW_HIDE = 0;
        private const int SW_SHOW = 5;
        private const int SW_RESTORE = 9;

        public WindowsTrayHelper(PhotinoWindow window)
        {
            _window = window;
        }

        public void Initialize()
        {
            _notifyIcon = new NotifyIcon();
            
            try 
            {
                // Try to extract icon from the running executable
                _notifyIcon.Icon = Icon.ExtractAssociatedIcon(System.Diagnostics.Process.GetCurrentProcess().MainModule.FileName);
            }
            catch 
            {
                // Fallback: System default (Error icon is usually available, or generic application)
                _notifyIcon.Icon = SystemIcons.Application;
            }

            _notifyIcon.Text = "Playerr - Web Server";
            _notifyIcon.Visible = true;

            // Context Menu
            var menu = new ContextMenuStrip();
            menu.Items.Add("Open Interface", null, (s, e) => RestoreWindow());
            menu.Items.Add("-");
            menu.Items.Add("Exit Playerr", null, (s, e) => ExitApp());
            _notifyIcon.ContextMenuStrip = menu;

            // Double Click -> Open
            _notifyIcon.DoubleClick += (s, e) => RestoreWindow();

            // Intercept Closing
            _window.WindowClosing += Window_WindowClosing;
        }

        private bool Window_WindowClosing(object sender, EventArgs e)
        {
            if (_isExplicitExit) 
            {
                return false; // Allow closing
            }

            // Hide to Tray instead of closing
            HideWindow();
            
            // Show a bubble tip once to inform user (optional)
            // _notifyIcon.ShowBalloonTip(3000, "Playerr Minimized", "App is running in the background.", ToolTipIcon.Info);
            
            return true; // Cancel closing
        }

        private void HideWindow()
        {
             ShowWindow(_window.WindowHandle, SW_HIDE);
        }

        private void RestoreWindow()
        {
             ShowWindow(_window.WindowHandle, SW_SHOW);
             // Also restore if minimized? SW_RESTORE handles it usually.
             ShowWindow(_window.WindowHandle, SW_RESTORE);
             // Ensure focus?
        }

        private void ExitApp()
        {
            _isExplicitExit = true;
            _notifyIcon.Visible = false;
            _window.Close();
        }

        public void Dispose()
        {
            _notifyIcon?.Dispose();
        }
    }
}
#endif
