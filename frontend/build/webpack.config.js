const path = require('path');
const HtmlWebpackPlugin = require('html-webpack-plugin');

module.exports = {
  mode: process.env.NODE_ENV === 'production' ? 'production' : 'development',
  entry: './frontend/src/index.tsx',
  output: {
    path: path.resolve(__dirname, '../../_output/UI'),
    filename: 'bundle.js',
    publicPath: '/'
  },
  resolve: {
    extensions: ['.ts', '.tsx', '.js', '.jsx']
  },
  module: {
    rules: [
      {
        test: /\.tsx?$/,
        use: {
          loader: 'ts-loader',
          options: {
            configFile: path.resolve(__dirname, 'tsconfig.json')
          }
        },
        exclude: /node_modules/
      },
      {
        test: /\.css$/,
        use: ['style-loader', 'css-loader']
      },
      {
        test: /\.(png|svg|jpg|jpeg|gif)$/i,
        type: 'asset/resource',
      }
    ]
  },
  plugins: [
    new HtmlWebpackPlugin({
      template: './frontend/src/index.html',
      filename: 'index.html',
      favicon: './frontend/src/assets/cargodeck-logo-white.svg',
      hash: true
    })
  ],
  devServer: {
    static: {
      directory: path.join(__dirname, '../../_output/UI')
    },
    historyApiFallback: true,
    port: 7878,
    hot: true,
    proxy: [
      {
        context: ['/api'],
        target: 'http://127.0.0.1:5002',
        changeOrigin: true,
        secure: false
      }
    ]
  }
};
